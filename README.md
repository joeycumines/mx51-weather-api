# mx51-weather-api

This technical exercise was approached from the perspective of an initial iteration on an actual production solution.
As a result, while the implementation is not production ready, an effort was made to produce code that would be able to
form part of the initial production release.

## Usage

```bash
# note that one or both may be provided
APP_OPENWEATHER_API_KEY='<your openweather api key>' \
APP_WEATHERSTACK_API_KEY='<your weatherstack api key>' \
go run github.com/joeycumines/mx51-weather-api/cmd/weather-api-standalone

# various cities are supported, but only sydney and brisbane have been tested
curl -s -i http://localhost:8080/v1/weather?city=sydney; echo
```

## Design Overview

The "planned" (i.e. not implemented) solution may be summarised as:

1. Public-facing service providing an [HTTP API](schema/v1/openapi.yaml) per the task specification
2. Internal service providing a [gRPC API](openweather/openweatherv1.proto) modeling openweather data (encapsulating
   auth and caching)
3. Internal service providing a [gRPC API](weatherstack/weatherstackv1.proto) modeling weatherstack data (encapsulating
   auth and caching)
4. Potentially service(s) and/or components to facilitate the desired caching behavior, for 3 and/or 4

The ideal caching _behavior_ would be similar to what was actually implemented, but distributed, scalable, and
fault-tolerant. Given the significant complexity, it's unlikely that such behavior would be attempted without a
demonstrated need, and as such a basic (race-prone) distributed stop-gap would be likely be implemented, instead.

## Protobuf and gRPC

Protobuf and gRPC are used by this project, primarily for internal APIs.
The protobuf package prefix `weather` has been used, and you can locate the schemas like
`find . -type f -name '*.proto'`.

Source code has been generated using relative paths for simplicity, but a more sophisticated method would be preferable
for production. Reasons for this include use cases such as generating more than just Go source, and leveraging more
sophisticated tools, e.g. [bufbuild/buf](https://github.com/bufbuild/buf).

## Implementation Notes

- The [internal/weather](internal/weather) package is the most production-ready
  code, and is unit tested somewhat thoroughly
- Everything else i.e. things under [cmd/weather-api-standalone](cmd/weather-api-standalone) are not for production
  as-is, and were slapped together, with an emphasis on demo-able behavior, in the interest of time
- Motivated by the observation that consistent behavior, across data sources, would be dependent on stable
  identification of locations, I had intended to do something with `weather.type.Location`, but ran out of time

There's plenty of discussion that could be had, e.g. around trade-offs and technology choices, but I'll leave it there
for now.
