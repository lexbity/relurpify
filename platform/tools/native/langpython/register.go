package langpython

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformpython "codeburg.org/lexbit/relurpify/platform/lang/python"
)

func init() {
	ports.RegisterNative("python_workspace_detect", func(basePath string) ports.Tool {
		return &platformpython.PythonWorkspaceDetectTool{BasePath: basePath}
	})
	ports.RegisterNative("python_project_metadata", func(basePath string) ports.Tool {
		return &platformpython.PythonProjectMetadataTool{BasePath: basePath}
	})
	ports.RegisterNative("python_pytest", func(basePath string) ports.Tool {
		return platformpython.NewPythonPytestTool(basePath)
	})
	ports.RegisterNative("python_unittest", func(basePath string) ports.Tool {
		return platformpython.NewPythonUnittestTool(basePath)
	})
	ports.RegisterNative("python_compile_check", func(basePath string) ports.Tool {
		return platformpython.NewPythonCompileCheckTool(basePath)
	})
}
