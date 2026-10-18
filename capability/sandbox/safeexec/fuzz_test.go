package safeexec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzPrepare drives the command sanitizer with arbitrary untrusted names and
// arguments. Invariants: preparation is total (never panics), the command name
// is always reduced to a separator-free base, each argument is normalized with
// filepath.Clean, and Command builds a non-nil *exec.Cmd whose argv carries
// exactly the sanitized values.
func FuzzPrepare(f *testing.F) {
	f.Add("/usr/bin/tool", "../escape/./path")
	f.Add("relative-name", "/abs/path")
	f.Add("", "")
	f.Add("a/b/c", "../../..")
	f.Add("name with spaces", "arg\x00with\xffbytes")
	f.Add("./././x", "//")

	f.Fuzz(func(t *testing.T, name, arg string) {
		prepared := Prepare(name, arg)

		assert.Equal(t, filepath.Base(name), prepared.Name)
		assert.NotContains(t, prepared.Name, "/",
			"sanitized command names must not contain path separators")

		require.Len(t, prepared.Args, 1)
		assert.Equal(t, filepath.Clean(arg), prepared.Args[0])

		cmd := Command(prepared)
		require.NotNil(t, cmd)
		require.Len(t, cmd.Args, 2)
		assert.Equal(t, prepared.Name, cmd.Args[0])
		assert.Equal(t, prepared.Args[0], cmd.Args[1])
		assert.Equal(t, resolvePath(prepared.Name), cmd.Path)

		// CommandContext must be constructible for the same input without
		// panicking; the context is never cancelled so the watcher goroutine
		// exits with the test process.
		ctx := t.Context()
		contextCmd := CommandContext(ctx, prepared)
		require.NotNil(t, contextCmd)
		assert.Equal(t, cmd.Path, contextCmd.Path)
	})
}
