package langgo

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformgo "codeburg.org/lexbit/relurpify/platform/lang/go"
)

func Constructors() map[string]ports.NativeToolConstructor {
	return map[string]ports.NativeToolConstructor{
		"go_workspace_detect": func(basePath string) ports.Tool { return &platformgo.GoWorkspaceDetectTool{BasePath: basePath} },
		"go_module_metadata":  func(basePath string) ports.Tool { return platformgo.NewGoModuleMetadataTool(basePath) },
		"go_test":             func(basePath string) ports.Tool { return platformgo.NewGoTestTool(basePath) },
		"go_build":            func(basePath string) ports.Tool { return platformgo.NewGoBuildTool(basePath) },
	}
}

func init() {
	for k, v := range Constructors() {
		ports.RegisterNative(k, v)
	}
}
