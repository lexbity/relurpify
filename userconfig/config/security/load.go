package security

import (
	"fmt"
	"os"
	"strings"
)

func loadAndDecode[T any](path, workspace string, decode Decoder, defaultPath func(string) string, out *T) error {
	if strings.TrimSpace(workspace) == "" {
		return fmt.Errorf("workspace required")
	}
	if strings.TrimSpace(path) == "" {
		path = defaultPath(workspace)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read policy %s: %w", path, err)
	}
	if decode == nil {
		return fmt.Errorf("decoder required")
	}
	if _, err := decode(path, data, out); err != nil {
		return err
	}
	return nil
}
