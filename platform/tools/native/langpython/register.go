package langpython

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformpython "codeburg.org/lexbit/relurpify/platform/lang/python"
)

func init() {
	contracts.RegisterNative("python_workspace_detect", func(basePath string) contracts.Tool {
		return &platformpython.PythonWorkspaceDetectTool{BasePath: basePath}
	})
	contracts.RegisterNative("python_project_metadata", func(basePath string) contracts.Tool {
		return &platformpython.PythonProjectMetadataTool{BasePath: basePath}
	})
	contracts.RegisterNative("python_pytest", func(basePath string) contracts.Tool {
		return platformpython.NewPythonPytestTool(basePath)
	})
	contracts.RegisterNative("python_unittest", func(basePath string) contracts.Tool {
		return platformpython.NewPythonUnittestTool(basePath)
	})
	contracts.RegisterNative("python_compile_check", func(basePath string) contracts.Tool {
		return platformpython.NewPythonCompileCheckTool(basePath)
	})
}
