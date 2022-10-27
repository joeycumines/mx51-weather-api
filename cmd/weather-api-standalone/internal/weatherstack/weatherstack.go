package weatherstack

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/joeycumines/go-bigbuff"
	"github.com/joeycumines/mx51-weather-api/weatherstack"
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

	cacheValue = weatherstack.CurrentWeather

	unimplementedServer = weatherstack.UnimplementedWeatherstackServer
)

var (
	// compile time assertions

	_ weatherstack.WeatherstackServer = (*Server)(nil)
)

func (x *Server) GetCurrentWeather(ctx context.Context, req *weatherstack.GetCurrentWeatherRequest) (*weatherstack.CurrentWeather, error) {
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
			log.Printf(`weatherstack request: %v`, req)
			res, err := x.getCurrentWeather(callCtx, req)
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
				log.Printf(`weatherstack response: %v`, v)
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
		return v.Result.(*weatherstack.CurrentWeather), nil
	}
}

func (x *Server) getCurrentWeather(ctx context.Context, request *weatherstack.GetCurrentWeatherRequest) (*weatherstack.CurrentWeather, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(
		`http://api.weatherstack.com/current?units=m&access_key=%s&query=%s`,
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
		Current struct {
			Temperature *float64 `json:"temperature"`
			WindSpeed   *float64 `json:"wind_speed"`
		} `json:"current"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Current.Temperature == nil {
		return nil, fmt.Errorf(`missing "current.temperature"`)
	}
	if body.Current.WindSpeed == nil {
		return nil, fmt.Errorf(`missing "current.wind_speed"`)
	}

	return &weatherstack.CurrentWeather{
		ReadTime:    timestamppb.New(readTime),
		Temperature: *body.Current.Temperature,
		WindSpeed:   *body.Current.WindSpeed,
	}, nil
}

func newCacheKey(req *weatherstack.GetCurrentWeatherRequest) cacheKey {
	return cacheKey{
		query: req.GetQuery(),
	}
}
