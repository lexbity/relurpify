// Package golang provides Go language tools for agents working on Go
// codebases.
//
// Registered capability tools include: go build, go test (with -run filter
// support), go vet, go fmt, go mod tidy, and go generate. Each tool runs via
// the sandbox runner and returns structured output — build errors, test
// results, and vet findings — as agent observations.
package golang
