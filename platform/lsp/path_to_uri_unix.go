//go:build !windows

package lsp

import "path/filepath"

func pathToURI(path string) string {
	path = filepath.Clean(path)
	if path == "" || path == "." {
		return "file:///"
	}
	if path[0] != '/' {
		path = "/" + path
	}
	return "file://" + path
}
