package fs

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformfs "codeburg.org/lexbit/relurpify/platform/fs"
)

// Constructors returns the fs tool constructors as an explicit map.
func Constructors() map[string]ports.NativeToolConstructor {
	return map[string]ports.NativeToolConstructor{
		"file_read":   func(basePath string) ports.Tool { return &platformfs.ReadFileTool{BasePath: basePath} },
		"file_write":  func(basePath string) ports.Tool { return &platformfs.WriteFileTool{BasePath: basePath} },
		"file_list":   func(basePath string) ports.Tool { return &platformfs.ListFilesTool{BasePath: basePath} },
		"file_search": func(basePath string) ports.Tool { return &platformfs.SearchInFilesTool{BasePath: basePath} },
		"file_create": func(basePath string) ports.Tool { return &platformfs.CreateFileTool{BasePath: basePath} },
		"file_delete": func(basePath string) ports.Tool { return &platformfs.DeleteFileTool{BasePath: basePath} },
	}
}

func init() {
	for k, v := range Constructors() {
		ports.RegisterNative(k, v)
	}
}
