package framework

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
)

var update = flag.Bool("update", false, "update golden files")

func goldenPath(name string) string {
	return filepath.Join("testdata", "golden", name)
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(goldenPath(name))
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v", name, err)
	}
	return string(data)
}

func writeGolden(t *testing.T, name, content string) {
	t.Helper()
	path := goldenPath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create golden dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write golden file %s: %v", name, err)
	}
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	if *update {
		writeGolden(t, name, got)
		return
	}
	want := readGolden(t, name)
	if got != want {
		t.Errorf("golden mismatch for %s\n--- got:\n%s\n--- want:\n%s", name, got, want)
	}
}

func TestInputBarGoldenRenderingDefault(t *testing.T) {
	bar := tui.NewInputBar()
	bar.SetWidth(80)
	rendered := bar.View("chat", false)
	assertGolden(t, "inputbar_default.txt", rendered)
}

func TestInputBarGoldenRenderingGated(t *testing.T) {
	bar := tui.NewInputBar()
	bar.SetWidth(80)
	bar.SetGated(true)
	rendered := bar.View("chat", false)
	assertGolden(t, "inputbar_gated.txt", rendered)
}

func TestInputBarGoldenRenderingStreaming(t *testing.T) {
	bar := tui.NewInputBar()
	bar.SetWidth(80)
	rendered := bar.View("chat", true)
	assertGolden(t, "inputbar_streaming.txt", rendered)
}

func TestInputBarNarrowWidth(t *testing.T) {
	bar := tui.NewInputBar()
	bar.SetWidth(40)
	rendered := bar.View("chat", false)
	if len(rendered) == 0 {
		t.Fatal("expected non-empty rendering at narrow width")
	}
}

func TestInputBarVeryNarrowWidth(t *testing.T) {
	bar := tui.NewInputBar()
	bar.SetWidth(10)
	rendered := bar.View("chat", false)
	if len(rendered) == 0 {
		t.Fatal("expected non-empty rendering at very narrow width")
	}
}
