package langjs

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformjs "codeburg.org/lexbit/relurpify/platform/lang/js"
)

func Constructors() map[string]ports.NativeToolConstructor {
	return map[string]ports.NativeToolConstructor{
		"node_workspace_detect": func(basePath string) ports.Tool { return &platformjs.NodeWorkspaceDetectTool{BasePath: basePath} },
		"node_project_metadata": func(basePath string) ports.Tool { return &platformjs.NodeProjectMetadataTool{BasePath: basePath} },
		"node_npm_test":         func(basePath string) ports.Tool { return platformjs.NewNodeNPMTestTool(basePath) },
		"node_syntax_check":     func(basePath string) ports.Tool { return platformjs.NewNodeSyntaxCheckTool(basePath) },
	}
}

func init() {
	for k, v := range Constructors() {
		ports.RegisterNative(k, v)
	}
}
