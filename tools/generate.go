// Package tools pins code generation tool dependencies so they are tracked
// in go.mod and go.sum and available via go tool.
//
// To regenerate protobuf types and ConnectRPC stubs, run from the repo root:
//
//	make generate
//
// or directly:
//
//	go tool buf generate
package tools
