package fs

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformfs "codeburg.org/lexbit/relurpify/platform/fs"
)

func init() {
	ports.RegisterNative("file_read", func(basePath string) ports.Tool {
		return &platformfs.ReadFileTool{BasePath: basePath}
	})
	ports.RegisterNative("file_write", func(basePath string) ports.Tool {
		return &platformfs.WriteFileTool{BasePath: basePath}
	})
	ports.RegisterNative("file_list", func(basePath string) ports.Tool {
		return &platformfs.ListFilesTool{BasePath: basePath}
	})
	ports.RegisterNative("file_search", func(basePath string) ports.Tool {
		return &platformfs.SearchInFilesTool{BasePath: basePath}
	})
	ports.RegisterNative("file_create", func(basePath string) ports.Tool {
		return &platformfs.CreateFileTool{BasePath: basePath}
	})
	ports.RegisterNative("file_delete", func(basePath string) ports.Tool {
		return &platformfs.DeleteFileTool{BasePath: basePath}
	})
}
