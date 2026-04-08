//go:build windows

package lsp

import (
	"path/filepath"
	"strings"
)

func pathToURI(path string) string {
	path = filepath.Clean(path)
	path = strings.ReplaceAll(path, "\\", "/")
	return "file:///" + strings.ReplaceAll(path, ":", "%3A")
}
