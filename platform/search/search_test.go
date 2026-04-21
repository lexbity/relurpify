package search

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
)

func TestGrepToolSkipsGeneratedDirectories(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "src", "main.rs")
	generated := filepath.Join(dir, "target", "debug", "artifact.txt")
	assert.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
	assert.NoError(t, os.MkdirAll(filepath.Dir(generated), 0o755))
	assert.NoError(t, os.WriteFile(source, []byte("compile error here\n"), 0o644))
	assert.NoError(t, os.WriteFile(generated, []byte("compile error in target\n"), 0o644))

	tool := &GrepTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"directory": ".",
		"pattern":   "error",
	})
	assert.NoError(t, err)
	bytes, err := json.Marshal(res.Data["matches"])
	assert.NoError(t, err)
	var decoded []map[string]interface{}
	assert.NoError(t, json.Unmarshal(bytes, &decoded))
	assert.Len(t, decoded, 1)
	assert.Equal(t, source, decoded[0]["file"])
}
