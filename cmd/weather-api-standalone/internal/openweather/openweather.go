package openweather

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/joeycumines/go-bigbuff"
	"github.com/joeycumines/mx51-weather-api/openweather"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type (
	Server struct {
		unimplementedServer

		APIKey string

		excl  bigbuff.Exclusive
		mu    sync.RWMutex
		cache map[cacheKey]*cacheValue
	}

	cacheKey struct {
		query string
	}

	cacheValue = openweather.Weather

	unimplementedServer = openweather.UnimplementedOpenweatherServer
)

var (
	// compile time assertions

	_ openweather.OpenweatherServer = (*Server)(nil)
)

func (x *Server) GetWeather(ctx context.Context, req *openweather.GetWeatherRequest) (*openweather.Weather, error) {
	key := newCacheKey(req)

	// fast path
	x.mu.RLock()
	if res := x.cache[key]; res != nil &&
		(req.GetMinReadTime() == nil ||
			(res.GetReadTime() != nil && !res.GetReadTime().AsTime().Before(req.GetMinReadTime().AsTime()))) {
		x.mu.RUnlock()
		return res, nil
	}
	x.mu.RUnlock()

	// summary:
	// - locked on the key
	// - "long poll" of 100ms prior to starting
	// - merge multiple concurrent calls (per key)
	// - rate limit (per key) to 500ms
	//
	// To do this in a distributed manner, you'd need to use something like kafka consumer groups, to schedule
	// and debounce the work, then a broadcast of the result (proper request-response might be possible, dunno, NATS
	// maybe).
	// TODO use a cancelable context
	callCtx := context.Background()
	ch := x.excl.CallWithOptions(
		bigbuff.ExclusiveKey(key),
		bigbuff.ExclusiveWait(time.Millisecond*100),
		bigbuff.ExclusiveRateLimit(callCtx, time.Millisecond*500),
		bigbuff.ExclusiveValue(func() (any, error) {
			log.Printf(`openweather request: %v`, req)
			res, err := x.getWeather(callCtx, req)
			if err == nil {
				x.mu.Lock()
				if x.cache == nil {
					x.cache = make(map[cacheKey]*cacheValue)
				}
				x.cache[key] = res
				x.mu.Unlock()
			}
			{
				var v any
				if err != nil {
					v = err
				} else {
					v = res
				}
				log.Printf(`openweather response: %v`, v)
			}
			return res, err
		}),
	)

	select {
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()

	case v := <-ch:
		if v.Error != nil {
			return nil, v.Error
		}
		return v.Result.(*openweather.Weather), nil
	}
}

func (x *Server) getWeather(ctx context.Context, request *openweather.GetWeatherRequest) (*openweather.Weather, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(
		`https://api.openweathermap.org/data/2.5/weather?units=metric&appid=%s&q=%s`,
		url.QueryEscape(x.APIKey),
		url.QueryEscape(request.GetQuery()),
	), nil)
	if err != nil {
		return nil, err
	}

	req = req.WithContext(ctx)

	readTime := time.Now()

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	defer io.Copy(io.Discard, res.Body)

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(`unexpected status code %d`, res.StatusCode)
	}

	var body struct {
		Main struct {
			Temp *float64 `json:"temp"`
		} `json:"main"`
		Wind struct {
			Speed *float64 `json:"speed"`
		} `json:"wind"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Main.Temp == nil {
		return nil, fmt.Errorf(`missing "main.temp"`)
	}
	if body.Wind.Speed == nil {
		return nil, fmt.Errorf(`missing "wind.speed"`)
	}

	return &openweather.Weather{
		ReadTime:  timestamppb.New(readTime),
		Temp:      *body.Main.Temp,
		WindSpeed: *body.Wind.Speed,
	}, nil
}

func newCacheKey(req *openweather.GetWeatherRequest) cacheKey {
	return cacheKey{
		query: req.GetQuery(),
	}
}
