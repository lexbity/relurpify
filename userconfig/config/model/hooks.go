package model

// ReadConfigFile is injected by cfgload during package initialization so the
// model loaders can use the workspace-safe file reader without importing cfgload.
var ReadConfigFile func(workspaceRoot, path string) ([]byte, error) //nolint:gochecknoglobals // overridable hook; nil unless wired by an embedder or tests

// RejectForbiddenSecretFields is injected by cfgload during package initialization.
var RejectForbiddenSecretFields func(path string, data []byte) error //nolint:gochecknoglobals // overridable hook; nil unless wired by an embedder or tests
