package langrust

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformrust "codeburg.org/lexbit/relurpify/platform/lang/rust"
)

func init() {
	ports.RegisterNative("rust_workspace_detect", func(basePath string) ports.Tool {
		return &platformrust.RustWorkspaceDetectTool{BasePath: basePath}
	})
	ports.RegisterNative("rust_cargo_test", func(basePath string) ports.Tool {
		return platformrust.NewRustCargoTestTool(basePath)
	})
	ports.RegisterNative("rust_cargo_check", func(basePath string) ports.Tool {
		return platformrust.NewRustCargoCheckTool(basePath)
	})
	ports.RegisterNative("rust_cargo_metadata", func(basePath string) ports.Tool {
		return platformrust.NewRustCargoMetadataTool(basePath)
	})
}
