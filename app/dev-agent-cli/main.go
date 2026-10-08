package main

// Build metadata reported by `dev-agent --version`. Release builds override
// these at link time (GoReleaser ldflags: -X main.version/commit/date); the
// values below are what a plain `go build` from source produces.
var (
	version = "dev"
	commit  = "none"    //nolint:gochecknoglobals // GoReleaser ldflags -X injection target; the linker can only set package-level vars
	date    = "unknown" //nolint:gochecknoglobals // GoReleaser ldflags -X injection target; the linker can only set package-level vars
)

func main() {
	Execute()
}
