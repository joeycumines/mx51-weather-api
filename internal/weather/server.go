package weather

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/joeycumines/mx51-weather-api/openweather"
	"github.com/joeycumines/mx51-weather-api/weatherstack"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http"
	"time"
)

type (
	// Server implements /v1/weather.
	// See also Register.
	Server struct {
		MaxAge       time.Duration
		TimeNow      func() time.Time
		Openweather  openweather.OpenweatherClient
		Weatherstack weatherstack.WeatherstackClient
	}

	weatherResponse struct {
		WindSpeed          float64 `json:"wind_speed"`
		TemperatureDegrees float64 `json:"temperature_degrees"`
	}
)

var (
	errMissingReadTime = errors.New(`missing read time`)
)

// Register wires up the server.
func (x *Server) Register(r chi.Router) {
	r.Get(`/v1/weather`, x.getWeather)
}

func (x *Server) getWeather(w http.ResponseWriter, r *http.Request) {
	// note: ignores potentially malformed query in request path
	params := r.URL.Query()

	// query built by parsing params (this would be far more interesting with multiple params)
	query := params.Get(`city`)
	if query == `` {
		_ = writeError(w, http.StatusBadRequest, status.Error(codes.InvalidArgument,
			`at least one query parameter required`))
		return
	}

	res, err := x.buildWeatherResponse(r.Context(), query)
	if err != nil {
		var statusCode int
		{
			sts, _ := status.FromError(err)
			switch sts.Code() {
			case codes.Unavailable:
				statusCode = http.StatusServiceUnavailable
			default:
				statusCode = http.StatusInternalServerError
			}
		}
		_ = writeError(w, statusCode, err)
		return
	}

	_ = writeJSON(w, http.StatusOK, res)
}

func (x *Server) buildWeatherResponse(ctx context.Context, query string) (*weatherResponse, error) {
	// TODO reconsider this, also consider independent timeouts for each sub-request
	ctx, cancel := context.WithTimeout(ctx, time.Minute*3)
	defer cancel()

	// a factory function accepting config would make this handling nicer
	if x.MaxAge <= 0 {
		panic(x.MaxAge)
	}
	minReadTime := x.TimeNow().Add(-x.MaxAge)

	// attempt providers in order of higher priority first

	owRes, owErr := x.Openweather.GetWeather(ctx, &openweather.GetWeatherRequest{Query: query})
	if owErr == nil {
		if owRes.GetReadTime() == nil {
			owErr = errMissingReadTime
		} else if !owRes.GetReadTime().AsTime().Before(minReadTime) {
			return new(weatherResponse).fromOpenweather(owRes), nil
		}
	}

	wsRes, wsErr := x.Weatherstack.GetCurrentWeather(ctx, &weatherstack.GetCurrentWeatherRequest{Query: query})
	if wsErr == nil {
		if wsRes.GetReadTime() == nil {
			wsErr = errMissingReadTime
		} else if !wsRes.GetReadTime().AsTime().Before(minReadTime) {
			return new(weatherResponse).fromWeatherstack(wsRes), nil
		}
	}

	// fallback to returning the freshest response
	switch {
	case owErr == nil && wsErr == nil:
		if wsRes.GetReadTime().AsTime().After(owRes.GetReadTime().AsTime()) {
			return new(weatherResponse).fromWeatherstack(wsRes), nil
		} else {
			return new(weatherResponse).fromOpenweather(owRes), nil
		}
	case owErr == nil:
		return new(weatherResponse).fromOpenweather(owRes), nil
	case wsErr == nil:
		return new(weatherResponse).fromWeatherstack(wsRes), nil
	default:
		return nil, status.Error(codes.Unavailable, `no weather providers available`)
	}
}

func (x *weatherResponse) fromOpenweather(res *openweather.Weather) *weatherResponse {
	x.WindSpeed = res.GetWindSpeed()
	x.TemperatureDegrees = res.GetTemp()
	return x
}

func (x *weatherResponse) fromWeatherstack(res *weatherstack.CurrentWeather) *weatherResponse {
	x.WindSpeed = res.GetWindSpeed()
	x.TemperatureDegrees = res.GetTemperature()
	return x
}
