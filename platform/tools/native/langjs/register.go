package langjs

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformjs "codeburg.org/lexbit/relurpify/platform/lang/js"
)

func init() {
	ports.RegisterNative("node_workspace_detect", func(basePath string) ports.Tool {
		return &platformjs.NodeWorkspaceDetectTool{BasePath: basePath}
	})
	ports.RegisterNative("node_project_metadata", func(basePath string) ports.Tool {
		return &platformjs.NodeProjectMetadataTool{BasePath: basePath}
	})
	ports.RegisterNative("node_npm_test", func(basePath string) ports.Tool {
		return platformjs.NewNodeNPMTestTool(basePath)
	})
	ports.RegisterNative("node_syntax_check", func(basePath string) ports.Tool {
		return platformjs.NewNodeSyntaxCheckTool(basePath)
	})
}
