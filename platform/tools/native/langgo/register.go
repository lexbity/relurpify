package langgo

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformgo "codeburg.org/lexbit/relurpify/platform/lang/go"
)

func init() {
	contracts.RegisterNative("go_workspace_detect", func(basePath string) contracts.Tool {
		return &platformgo.GoWorkspaceDetectTool{BasePath: basePath}
	})
	contracts.RegisterNative("go_module_metadata", func(basePath string) contracts.Tool {
		return platformgo.NewGoModuleMetadataTool(basePath)
	})
	contracts.RegisterNative("go_test", func(basePath string) contracts.Tool {
		return platformgo.NewGoTestTool(basePath)
	})
	contracts.RegisterNative("go_build", func(basePath string) contracts.Tool {
		return platformgo.NewGoBuildTool(basePath)
	})
}
