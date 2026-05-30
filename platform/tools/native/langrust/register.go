package langrust

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformrust "codeburg.org/lexbit/relurpify/platform/lang/rust"
)

func init() {
	contracts.RegisterNative("rust_workspace_detect", func(basePath string) contracts.Tool {
		return &platformrust.RustWorkspaceDetectTool{BasePath: basePath}
	})
	contracts.RegisterNative("rust_cargo_test", func(basePath string) contracts.Tool {
		return platformrust.NewRustCargoTestTool(basePath)
	})
	contracts.RegisterNative("rust_cargo_check", func(basePath string) contracts.Tool {
		return platformrust.NewRustCargoCheckTool(basePath)
	})
	contracts.RegisterNative("rust_cargo_metadata", func(basePath string) contracts.Tool {
		return platformrust.NewRustCargoMetadataTool(basePath)
	})
}
