package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvVarDocsMatchRecognizedEnvVars(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	data, err := os.ReadFile(filepath.Join(root, "docs", "env-vars.md"))
	require.NoError(t, err)

	var got []string
	inList := false
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "## Recognized Variables"):
			inList = true
		case strings.HasPrefix(line, "## ") && inList:
			inList = false
		case inList && strings.HasPrefix(strings.TrimSpace(line), "- `"):
			value := strings.TrimSpace(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- `")))
			value = strings.TrimSuffix(value, "`")
			if value != "" {
				got = append(got, value)
			}
		}
	}

	require.Equal(t, RecognizedEnvVars(), got)
}
