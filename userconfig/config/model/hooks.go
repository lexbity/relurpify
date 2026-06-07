package model

// ReadConfigFile is injected by cfgload during package initialization so the
// model loaders can use the workspace-safe file reader without importing cfgload.
var ReadConfigFile func(workspaceRoot, path string) ([]byte, error)

// RejectForbiddenSecretFields is injected by cfgload during package initialization.
var RejectForbiddenSecretFields func(path string, data []byte) error
