package langjs

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformjs "codeburg.org/lexbit/relurpify/platform/lang/js"
)

func init() {
	contracts.RegisterNative("node_workspace_detect", func(basePath string) contracts.Tool {
		return &platformjs.NodeWorkspaceDetectTool{BasePath: basePath}
	})
	contracts.RegisterNative("node_project_metadata", func(basePath string) contracts.Tool {
		return &platformjs.NodeProjectMetadataTool{BasePath: basePath}
	})
	contracts.RegisterNative("node_npm_test", func(basePath string) contracts.Tool {
		return platformjs.NewNodeNPMTestTool(basePath)
	})
	contracts.RegisterNative("node_syntax_check", func(basePath string) contracts.Tool {
		return platformjs.NewNodeSyntaxCheckTool(basePath)
	})
}
