package weather

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/joeycumines/mx51-weather-api/openweather"
	locationpb "github.com/joeycumines/mx51-weather-api/type/location"
	"github.com/joeycumines/mx51-weather-api/weatherstack"
	latlongpb "google.golang.org/genproto/googleapis/type/latlng"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

var (
	sydLocation = &locationpb.Location{
		Name: `Sydney`,
		Position: &latlongpb.LatLng{
			Latitude:  -33.8688,
			Longitude: 151.2093,
		},
	}
)

func TestServer(t *testing.T) {
	t.Parallel()

	type (
		OpenweatherRequest struct {
			ctx context.Context
			req *openweather.GetWeatherRequest
		}
		OpenweatherResponse struct {
			res *openweather.Weather
			err error
		}
		WeatherstackRequest struct {
			ctx context.Context
			req *weatherstack.GetCurrentWeatherRequest
		}
		WeatherstackResponse struct {
			res *weatherstack.CurrentWeather
			err error
		}
		Harness struct {
			ts              *httptest.Server
			setTime         func(t time.Time)
			openweatherIn   <-chan OpenweatherRequest
			openweatherOut  chan<- OpenweatherResponse
			weatherstackIn  <-chan WeatherstackRequest
			weatherstackOut chan<- WeatherstackResponse
		}
	)

	for _, tc := range [...]struct {
		name   string
		maxAge time.Duration
		test   func(t *testing.T, h Harness)
	}{
		{
			name: `input validation`,
			test: func(t *testing.T, h Harness) {
				t.Run(`no query params`, func(t *testing.T) {
					t.Parallel()
					res, body := testRequest(t, h.ts, http.MethodGet, `/v1/weather`, nil)
					if res.StatusCode != http.StatusBadRequest {
						t.Errorf(`unexpected status code: %d`, res.StatusCode)
					}
					if v := res.Header.Get(`Content-Type`); v != `application/json` {
						t.Errorf(`unexpected content type: %s`, v)
					}
					if v := res.Header.Get(`Content-Length`); v != strconv.Itoa(len(body)) {
						t.Errorf(`unexpected content length: %s`, v)
					}
					if body != `{"code":3,"message":"at least one query parameter required"}` {
						t.Errorf("unexpected body: %q\n%s", body, body)
					}
				})
				t.Run(`invalid path`, func(t *testing.T) {
					t.Parallel()
					res, body := testRequest(t, h.ts, http.MethodGet, `/v1/nah`, nil)
					if res.StatusCode != http.StatusNotFound {
						t.Errorf(`unexpected status code: %d`, res.StatusCode)
					}
					if body != "404 page not found\n" {
						t.Errorf("unexpected body: %q\n%s", body, body)
					}
				})
				t.Run(`invalid method`, func(t *testing.T) {
					t.Parallel()
					res, _ := testRequest(t, h.ts, http.MethodDelete, `/v1/weather`, nil)
					if res.StatusCode != http.StatusMethodNotAllowed {
						t.Errorf(`unexpected status code: %d`, res.StatusCode)
					}
				})
			},
		},
		{
			name:   `openweather weatherstack`,
			maxAge: time.Second * 3,
			test: func(t *testing.T, h Harness) {
				// note these tests aren't parallel, since the harness state matters (request ping/pong + time)

				setTime := func(d time.Duration) { h.setTime(time.Unix(0, int64(d))) }

				type TestResult struct {
					res  *http.Response
					body string
				}
				testRequest := func(t *testing.T, ts *httptest.Server, method, path string, body io.Reader) <-chan TestResult {
					ch := make(chan TestResult, 1)
					go func() {
						res, body := testRequest(t, ts, method, path, body)
						ch <- TestResult{res, body}
					}()
					return ch
				}

				t.Run(`openweather success`, func(t *testing.T) {
					setTime(0)
					ch := testRequest(t, h.ts, http.MethodGet, `/v1/weather?city=sydney`, nil)
					{
						req := <-h.openweatherIn
						if query := req.req.GetQuery(); query != `sydney` {
							t.Errorf(`unexpected query: %q`, query)
						}
						h.openweatherOut <- OpenweatherResponse{res: &openweather.Weather{
							ReadTime:  timestamppb.New(time.Unix(0, int64(time.Millisecond*25))),
							Location:  sydLocation,
							Temp:      29,
							WindSpeed: 20,
						}}
					}
					out := <-ch
					if out.res.StatusCode != http.StatusOK {
						t.Errorf(`unexpected status code: %d`, out.res.StatusCode)
					}
					if v := out.res.Header.Get(`Content-Type`); v != `application/json` {
						t.Errorf(`unexpected content type: %s`, v)
					}
					if v := out.res.Header.Get(`Content-Length`); v != strconv.Itoa(len(out.body)) {
						t.Errorf(`unexpected content length: %s`, v)
					}
					if out.body != `{"wind_speed":72,"temperature_degrees":29}` {
						t.Errorf("unexpected body: %q\n%s", out.body, out.body)
					}
				})

				t.Run(`openweather cached`, func(t *testing.T) {
					setTime(0)
					ch := testRequest(t, h.ts, http.MethodGet, `/v1/weather?city=brisbane`, nil)
					{
						req := <-h.openweatherIn
						if query := req.req.GetQuery(); query != `brisbane` {
							t.Errorf(`unexpected query: %q`, query)
						}
						if minReadTime := req.req.GetMinReadTime(); minReadTime == nil || !minReadTime.AsTime().Equal(time.Unix(0, 0).Add(-time.Second*3)) {
							t.Errorf(`unexpected min read time: %q`, minReadTime)
						}
						h.openweatherOut <- OpenweatherResponse{res: &openweather.Weather{
							ReadTime:  timestamppb.New(time.Unix(0, int64(time.Second*-2))),
							Temp:      23,
							WindSpeed: 15,
						}}
					}
					out := <-ch
					if out.res.StatusCode != http.StatusOK {
						t.Errorf(`unexpected status code: %d`, out.res.StatusCode)
					}
					if out.body != `{"wind_speed":54,"temperature_degrees":23}` {
						t.Errorf("unexpected body: %q\n%s", out.body, out.body)
					}
				})

				t.Run(`openweather expired weatherstack success`, func(t *testing.T) {
					setTime(0)
					ch := testRequest(t, h.ts, http.MethodGet, `/v1/weather?city=sydney`, nil)
					<-h.openweatherIn
					h.openweatherOut <- OpenweatherResponse{res: &openweather.Weather{
						ReadTime:  timestamppb.New(time.Unix(0, -int64(time.Minute*3+1))),
						Location:  sydLocation,
						Temp:      33,
						WindSpeed: 3,
					}}
					{
						req := <-h.weatherstackIn
						if query := req.req.GetQuery(); query != `sydney` {
							t.Errorf(`unexpected query: %q`, query)
						}
						if minReadTime := req.req.GetMinReadTime(); minReadTime == nil || !minReadTime.AsTime().Equal(time.Unix(0, 0).Add(-time.Second*3)) {
							t.Errorf(`unexpected min read time: %q`, minReadTime)
						}
						h.weatherstackOut <- WeatherstackResponse{res: &weatherstack.CurrentWeather{
							ReadTime:    timestamppb.New(time.Unix(0, int64(time.Millisecond*25))),
							Location:    sydLocation,
							Temperature: 29,
							WindSpeed:   20,
						}}
					}
					out := <-ch
					if out.res.StatusCode != http.StatusOK {
						t.Errorf(`unexpected status code: %d`, out.res.StatusCode)
					}
					if out.body != "{\"wind_speed\":20,\"temperature_degrees\":29}" {
						t.Errorf("unexpected body: %q\n%s", out.body, out.body)
					}
				})

				t.Run(`openweather error weatherstack success`, func(t *testing.T) {
					setTime(0)
					ch := testRequest(t, h.ts, http.MethodGet, `/v1/weather?city=sydney`, nil)
					<-h.openweatherIn
					h.openweatherOut <- OpenweatherResponse{err: errors.New(`openweather error`)}
					<-h.weatherstackIn
					h.weatherstackOut <- WeatherstackResponse{res: &weatherstack.CurrentWeather{
						ReadTime:    timestamppb.New(time.Unix(0, int64(time.Millisecond*25))),
						Location:    sydLocation,
						Temperature: 29,
						WindSpeed:   20,
					}}
					out := <-ch
					if out.res.StatusCode != http.StatusOK {
						t.Errorf(`unexpected status code: %d`, out.res.StatusCode)
					}
					if out.body != "{\"wind_speed\":20,\"temperature_degrees\":29}" {
						t.Errorf("unexpected body: %q\n%s", out.body, out.body)
					}
				})

				t.Run(`both error`, func(t *testing.T) {
					setTime(0)
					ch := testRequest(t, h.ts, http.MethodGet, `/v1/weather?city=sydney`, nil)
					<-h.openweatherIn
					h.openweatherOut <- OpenweatherResponse{err: errors.New(`openweather error`)}
					<-h.weatherstackIn
					h.weatherstackOut <- WeatherstackResponse{err: errors.New(`weatherstack error`)}
					out := <-ch
					if out.res.StatusCode != http.StatusServiceUnavailable {
						t.Errorf(`unexpected status code: %d`, out.res.StatusCode)
					}
					if out.body != `{"code":14,"message":"no weather providers available"}` {
						t.Errorf("unexpected body: %q\n%s", out.body, out.body)
					}
				})

				t.Run(`both expired same read time`, func(t *testing.T) {
					setTime(0)
					expiredReadTime := timestamppb.New(time.Unix(0, -int64(time.Minute*3+1)))
					ch := testRequest(t, h.ts, http.MethodGet, `/v1/weather?city=sydney`, nil)
					<-h.openweatherIn
					h.openweatherOut <- OpenweatherResponse{res: &openweather.Weather{
						ReadTime:  expiredReadTime,
						Location:  sydLocation,
						Temp:      33,
						WindSpeed: 3,
					}}
					<-h.weatherstackIn
					h.weatherstackOut <- WeatherstackResponse{res: &weatherstack.CurrentWeather{
						ReadTime:    expiredReadTime,
						Location:    sydLocation,
						Temperature: 29,
						WindSpeed:   20,
					}}
					out := <-ch
					if out.res.StatusCode != http.StatusOK {
						t.Errorf(`unexpected status code: %d`, out.res.StatusCode)
					}
					if out.body != `{"wind_speed":10.8,"temperature_degrees":33}` {
						t.Errorf("unexpected body: %q\n%s", out.body, out.body)
					}
				})

				t.Run(`both expired weatherstack earlier`, func(t *testing.T) {
					setTime(0)
					ch := testRequest(t, h.ts, http.MethodGet, `/v1/weather?city=sydney`, nil)
					<-h.openweatherIn
					h.openweatherOut <- OpenweatherResponse{res: &openweather.Weather{
						ReadTime:  timestamppb.New(time.Unix(0, -int64(time.Minute*3+2))),
						Location:  sydLocation,
						Temp:      33,
						WindSpeed: 3,
					}}
					<-h.weatherstackIn
					h.weatherstackOut <- WeatherstackResponse{res: &weatherstack.CurrentWeather{
						ReadTime:    timestamppb.New(time.Unix(0, -int64(time.Minute*3+1))),
						Location:    sydLocation,
						Temperature: 29,
						WindSpeed:   20,
					}}
					out := <-ch
					if out.res.StatusCode != http.StatusOK {
						t.Errorf(`unexpected status code: %d`, out.res.StatusCode)
					}
					if out.body != `{"wind_speed":20,"temperature_degrees":29}` {
						t.Errorf("unexpected body: %q\n%s", out.body, out.body)
					}
				})

				t.Run(`openweather expired weatherstack error`, func(t *testing.T) {
					setTime(0)
					ch := testRequest(t, h.ts, http.MethodGet, `/v1/weather?city=sydney`, nil)
					<-h.openweatherIn
					h.openweatherOut <- OpenweatherResponse{res: &openweather.Weather{
						ReadTime:  timestamppb.New(time.Unix(0, -int64(time.Minute*3+2))),
						Location:  sydLocation,
						Temp:      33,
						WindSpeed: 3,
					}}
					<-h.weatherstackIn
					h.weatherstackOut <- WeatherstackResponse{err: errors.New(`weatherstack error`)}
					out := <-ch
					if out.res.StatusCode != http.StatusOK {
						t.Errorf(`unexpected status code: %d`, out.res.StatusCode)
					}
					if out.body != `{"wind_speed":10.8,"temperature_degrees":33}` {
						t.Errorf("unexpected body: %q\n%s", out.body, out.body)
					}
				})

				t.Run(`openweather error weatherstack expired`, func(t *testing.T) {
					setTime(0)
					ch := testRequest(t, h.ts, http.MethodGet, `/v1/weather?city=sydney`, nil)
					<-h.openweatherIn
					h.openweatherOut <- OpenweatherResponse{err: errors.New(`openweather error`)}
					<-h.weatherstackIn
					h.weatherstackOut <- WeatherstackResponse{res: &weatherstack.CurrentWeather{
						ReadTime:    timestamppb.New(time.Unix(0, -int64(time.Minute*3+1))),
						Location:    sydLocation,
						Temperature: 29,
						WindSpeed:   20,
					}}
					out := <-ch
					if out.res.StatusCode != http.StatusOK {
						t.Errorf(`unexpected status code: %d`, out.res.StatusCode)
					}
					if out.body != `{"wind_speed":20,"temperature_degrees":29}` {
						t.Errorf("unexpected body: %q\n%s", out.body, out.body)
					}
				})
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			openweatherIn := make(chan OpenweatherRequest)
			openweatherOut := make(chan OpenweatherResponse)
			weatherstackIn := make(chan WeatherstackRequest)
			weatherstackOut := make(chan WeatherstackResponse)

			getTime, setTime := mockTime()

			server := Server{
				MaxAge:  tc.maxAge,
				TimeNow: getTime,
				Openweather: &mockOpenweatherClient{getWeather: func(ctx context.Context, in *openweather.GetWeatherRequest, _ ...grpc.CallOption) (*openweather.Weather, error) {
					openweatherIn <- OpenweatherRequest{ctx: ctx, req: in}
					out := <-openweatherOut
					return out.res, out.err
				}},
				Weatherstack: &mockWeatherstackClient{getCurrentWeather: func(ctx context.Context, in *weatherstack.GetCurrentWeatherRequest, _ ...grpc.CallOption) (*weatherstack.CurrentWeather, error) {
					weatherstackIn <- WeatherstackRequest{ctx: ctx, req: in}
					out := <-weatherstackOut
					return out.res, out.err
				}},
			}

			router := chi.NewRouter()
			router.Route(`/`, server.Register)

			ts := httptest.NewServer(router)
			t.Cleanup(ts.Close)

			tc.test(t, Harness{
				ts:              ts,
				setTime:         setTime,
				openweatherIn:   openweatherIn,
				openweatherOut:  openweatherOut,
				weatherstackIn:  weatherstackIn,
				weatherstackOut: weatherstackOut,
			})
		})
	}
}
