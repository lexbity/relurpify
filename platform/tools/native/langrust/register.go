package langrust

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformrust "codeburg.org/lexbit/relurpify/platform/lang/rust"
)

func Constructors() map[string]ports.NativeToolConstructor {
	return map[string]ports.NativeToolConstructor{
		"rust_workspace_detect": func(basePath string) ports.Tool { return &platformrust.RustWorkspaceDetectTool{BasePath: basePath} },
		"rust_cargo_test":       func(basePath string) ports.Tool { return platformrust.NewRustCargoTestTool(basePath) },
		"rust_cargo_check":      func(basePath string) ports.Tool { return platformrust.NewRustCargoCheckTool(basePath) },
		"rust_cargo_metadata":   func(basePath string) ports.Tool { return platformrust.NewRustCargoMetadataTool(basePath) },
	}
}

func init() {
	for k, v := range Constructors() {
		ports.RegisterNative(k, v)
	}
}
