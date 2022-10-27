package weather

import (
	"context"
	"github.com/joeycumines/mx51-weather-api/openweather"
	"github.com/joeycumines/mx51-weather-api/weatherstack"
	"google.golang.org/grpc"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type (
	mockOpenweatherClient struct {
		getWeather func(ctx context.Context, in *openweather.GetWeatherRequest, opts ...grpc.CallOption) (*openweather.Weather, error)
	}

	mockWeatherstackClient struct {
		getCurrentWeather func(ctx context.Context, in *weatherstack.GetCurrentWeatherRequest, opts ...grpc.CallOption) (*weatherstack.CurrentWeather, error)
	}
)

var (
	// compile time assertions

	_ openweather.OpenweatherClient   = (*mockOpenweatherClient)(nil)
	_ weatherstack.WeatherstackClient = (*mockWeatherstackClient)(nil)
)

func (x *mockOpenweatherClient) GetWeather(ctx context.Context, in *openweather.GetWeatherRequest, opts ...grpc.CallOption) (*openweather.Weather, error) {
	return x.getWeather(ctx, in, opts...)
}

func (x *mockWeatherstackClient) GetCurrentWeather(ctx context.Context, in *weatherstack.GetCurrentWeatherRequest, opts ...grpc.CallOption) (*weatherstack.CurrentWeather, error) {
	return x.getCurrentWeather(ctx, in, opts...)
}

func mockTime() (get func() time.Time, set func(t time.Time)) {
	var (
		mu  sync.RWMutex
		now time.Time
	)
	get = func() time.Time {
		mu.RLock()
		defer mu.RUnlock()
		if now == (time.Time{}) {
			return time.Now()
		}
		return now
	}
	set = func(t time.Time) {
		mu.Lock()
		defer mu.Unlock()
		now = t
	}
	return
}

func testRequest(t *testing.T, ts *httptest.Server, method, path string, body io.Reader) (*http.Response, string) {
	req, err := http.NewRequest(method, ts.URL+path, body)
	if err != nil {
		t.Fatal(err)
		return nil, ""
	}

	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
		return nil, ""
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
		return nil, ""
	}
	defer res.Body.Close()

	return res, string(resBody)
}
