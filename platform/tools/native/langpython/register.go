package langpython

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformpython "codeburg.org/lexbit/relurpify/platform/lang/python"
)

func Constructors() map[string]ports.NativeToolConstructor {
	return map[string]ports.NativeToolConstructor{
		"python_workspace_detect": func(basePath string) ports.Tool { return &platformpython.PythonWorkspaceDetectTool{BasePath: basePath} },
		"python_project_metadata": func(basePath string) ports.Tool { return &platformpython.PythonProjectMetadataTool{BasePath: basePath} },
		"python_pytest":           func(basePath string) ports.Tool { return platformpython.NewPythonPytestTool(basePath) },
		"python_unittest":         func(basePath string) ports.Tool { return platformpython.NewPythonUnittestTool(basePath) },
		"python_compile_check":    func(basePath string) ports.Tool { return platformpython.NewPythonCompileCheckTool(basePath) },
	}
}

func init() {
	for k, v := range Constructors() {
		ports.RegisterNative(k, v)
	}
}
