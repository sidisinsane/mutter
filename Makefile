.PHONY: generate build tidy

## generate: Run buf code generation for protobuf types and ConnectRPC stubs.
## Must be run from the repository root.
generate:
	go tool buf generate

## build: Build all binaries.
build:
	go build ./...

## tidy: Tidy go.mod and go.sum.
tidy:
	go mod tidy
