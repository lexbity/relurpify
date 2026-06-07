package langgo

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformgo "codeburg.org/lexbit/relurpify/platform/lang/go"
)

func init() {
	ports.RegisterNative("go_workspace_detect", func(basePath string) ports.Tool {
		return &platformgo.GoWorkspaceDetectTool{BasePath: basePath}
	})
	ports.RegisterNative("go_module_metadata", func(basePath string) ports.Tool {
		return platformgo.NewGoModuleMetadataTool(basePath)
	})
	ports.RegisterNative("go_test", func(basePath string) ports.Tool {
		return platformgo.NewGoTestTool(basePath)
	})
	ports.RegisterNative("go_build", func(basePath string) ports.Tool {
		return platformgo.NewGoBuildTool(basePath)
	})
}
