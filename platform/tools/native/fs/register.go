package fs

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformfs "codeburg.org/lexbit/relurpify/platform/fs"
)

func init() {
	contracts.RegisterNative("file_read", func(basePath string) contracts.Tool {
		return &platformfs.ReadFileTool{BasePath: basePath}
	})
	contracts.RegisterNative("file_write", func(basePath string) contracts.Tool {
		return &platformfs.WriteFileTool{BasePath: basePath}
	})
	contracts.RegisterNative("file_list", func(basePath string) contracts.Tool {
		return &platformfs.ListFilesTool{BasePath: basePath}
	})
	contracts.RegisterNative("file_search", func(basePath string) contracts.Tool {
		return &platformfs.SearchInFilesTool{BasePath: basePath}
	})
	contracts.RegisterNative("file_create", func(basePath string) contracts.Tool {
		return &platformfs.CreateFileTool{BasePath: basePath}
	})
	contracts.RegisterNative("file_delete", func(basePath string) contracts.Tool {
		return &platformfs.DeleteFileTool{BasePath: basePath}
	})
}
