package main

import (
	"github.com/fullstorydev/grpchan"
	"github.com/fullstorydev/grpchan/inprocgrpc"
	"github.com/go-chi/chi/v5"
	owapi "github.com/joeycumines/mx51-weather-api/cmd/weather-api-standalone/internal/openweather"
	wsapi "github.com/joeycumines/mx51-weather-api/cmd/weather-api-standalone/internal/weatherstack"
	"github.com/joeycumines/mx51-weather-api/internal/weather"
	"github.com/joeycumines/mx51-weather-api/openweather"
	"github.com/joeycumines/mx51-weather-api/weatherstack"
	"net/http"
	"os"
	"time"
)

func main() {
	// init (in-process) gRPC server implementations for the weather apis
	// note: these would be in separate (load balanced, redundant) processes, in a real world scenario
	handlers := make(grpchan.HandlerMap)
	if key := os.Getenv(`APP_OPENWEATHER_API_KEY`); key != `` {
		openweather.RegisterOpenweatherServer(handlers, &owapi.Server{
			APIKey: key,
		})
	}
	if key := os.Getenv(`APP_WEATHERSTACK_API_KEY`); key != `` {
		weatherstack.RegisterWeatherstackServer(handlers, &wsapi.Server{
			APIKey: key,
		})
	}
	if len(handlers) == 0 {
		panic(`no api keys provided`)
	}

	// the actual in-process gRPC client
	var conn inprocgrpc.Channel
	handlers.ForEach(conn.RegisterService)

	server := weather.Server{
		MaxAge:       time.Second * 3,
		TimeNow:      time.Now,
		Openweather:  openweather.NewOpenweatherClient(&conn),
		Weatherstack: weatherstack.NewWeatherstackClient(&conn),
	}

	router := chi.NewRouter()
	router.Route(`/`, server.Register)

	panic(http.ListenAndServe(`:8080`, router))
}
