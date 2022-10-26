//go:build tools
// +build tools

package tools

import (
	_ "golang.org/x/tools/cmd/godoc"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
	_ "honnef.co/go/tools/cmd/staticcheck"
)
