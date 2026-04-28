package search

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

func shouldSkipGeneratedDir(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	switch name {
	case ".git", "target", "node_modules", "dist", "build":
		return true
	default:
		return false
	}
}

func preparePath(base, path string) string {
	if base == "" {
		return filepath.Clean(path)
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

const scanChunkSize = 64 * 1024

func scanLinesOrChunks(maxChunk int) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		limit := len(data)
		if limit > maxChunk {
			limit = maxChunk
		}
		if i := bytes.IndexByte(data[:limit], '\n'); i >= 0 {
			line := data[:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			return i + 1, line, nil
		}
		if len(data) >= maxChunk {
			return maxChunk, data[:maxChunk], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
