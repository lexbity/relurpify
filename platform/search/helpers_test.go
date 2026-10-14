package search

import (
	"bufio"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldSkipGeneratedDir(t *testing.T) {
	cases := []struct {
		name string
		dir  string
		want bool
	}{
		{"git dir", ".git", true},
		{"rust target", "target", true},
		{"node modules", "node_modules", true},
		{"dist", "dist", true},
		{"build", "build", true},
		{"padded name", "  .git  ", true},
		{"source dir", "src", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.dir, func(t *testing.T) {
			require.Equal(t, tc.want, shouldSkipGeneratedDir(tc.dir))
		})
	}
}

func TestPreparePath(t *testing.T) {
	require.Equal(t, "a/b/b.txt", preparePath("", "a/b/./b.txt"))
	require.Equal(t, "/abs/path", preparePath("base", "/abs/path"))
	require.Equal(t, filepath.Join("base", "rel.txt"), preparePath("base", "rel.txt"))
}

func TestResolveWithinBase(t *testing.T) {
	base := t.TempDir()

	resolved, err := resolveWithinBase(base, ".")
	require.NoError(t, err)
	require.Equal(t, base, resolved, "dot resolves to the base itself")

	_, err = resolveWithinBase(base, "../outside")
	require.Error(t, err, "parent traversal must be rejected")

	_, err = resolveWithinBase(base, "")
	require.Error(t, err, "empty candidate must be rejected")
}

func TestScanLinesOrChunks_SplitsLinesAndStripsCR(t *testing.T) {
	scan := func(input string) []string {
		sc := bufio.NewScanner(strings.NewReader(input))
		sc.Buffer(make([]byte, scanChunkSize), scanChunkSize)
		sc.Split(scanLinesOrChunks(scanChunkSize))
		var tokens []string
		for sc.Scan() {
			tokens = append(tokens, sc.Text())
		}
		require.NoError(t, sc.Err())
		return tokens
	}

	require.Equal(t, []string{"a", "b", "c"}, scan("a\nb\r\nc"))
	require.Equal(t, []string{"no trailing newline"}, scan("no trailing newline"))
	require.Nil(t, scan(""))
	require.Equal(t, []string{"", "", ""}, scan("\n\n\n"))
}

func TestScanLinesOrChunks_ChunksOverlongLines(t *testing.T) {
	long := strings.Repeat("x", 3*scanChunkSize) + "tail-without-newline"
	sc := bufio.NewScanner(strings.NewReader(long))
	sc.Buffer(make([]byte, scanChunkSize), scanChunkSize)
	sc.Split(scanLinesOrChunks(scanChunkSize))

	var tokens []string
	for sc.Scan() {
		tokens = append(tokens, sc.Text())
	}
	require.NoError(t, sc.Err())

	// Every token is at most one chunk; consecutive chunk tokens reconstruct
	// the input because no newline ever appears.
	var rebuilt strings.Builder
	for _, tok := range tokens {
		require.LessOrEqual(t, len(tok), scanChunkSize)
		rebuilt.WriteString(tok)
	}
	require.Equal(t, long, rebuilt.String())
}

func FuzzScanLinesOrChunks(f *testing.F) {
	f.Add("")
	f.Add("single line")
	f.Add("a\r\nb\rc\n")
	f.Add("\n\n\n")
	f.Add(strings.Repeat("chunky ", scanChunkSize/4))
	f.Add("x\x00y\xff\xfe\nbinary-ish\n")

	f.Fuzz(func(t *testing.T, input string) {
		sc := bufio.NewScanner(strings.NewReader(input))
		sc.Buffer(make([]byte, scanChunkSize), scanChunkSize)
		sc.Split(scanLinesOrChunks(scanChunkSize))

		var rebuilt strings.Builder
		for sc.Scan() {
			tok := sc.Text()
			require.LessOrEqual(t, len(tok), scanChunkSize, "token must never exceed the chunk size")
			rebuilt.WriteString(tok)
		}
		require.NoError(t, sc.Err())
	})
}
