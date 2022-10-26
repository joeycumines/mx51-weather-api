# mx51-weather-api

## Protobuf and gRPC

Protobuf and gRPC are used by this project, primarily for internal APIs.
The protobuf package prefix `weather` has been used, and you can locate the schemas like
`find . -type f -name '*.proto'`.

Source code has been generated using relative paths for simplicity, but a more sophisticated method would be preferable
for production. Reasons for this include use cases such as generating more than just Go source, and leveraging more
sophisticated tools, e.g. [bufbuild/buf](https://github.com/bufbuild/buf).
