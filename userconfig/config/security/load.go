package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readPolicy(path, workspace string, defaultPath func(string) string) ([]byte, string, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, "", fmt.Errorf("workspace required")
	}
	if strings.TrimSpace(path) == "" {
		path = defaultPath(workspace)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, "", fmt.Errorf("read policy %s: %w", path, err)
	}
	return data, path, nil
}

func decodePolicy[T any](path string, data []byte, decode Decoder, out *T) error {
	if decode == nil {
		return fmt.Errorf("decoder required")
	}
	if _, err := decode(path, data, out); err != nil {
		return err
	}
	return nil
}
