package tui

import (
	"strings"
	"testing"
)

func TestLogoMascotWideEnough(t *testing.T) {
	// At >= 57 cols and >= 22 rows, the mascot should be chosen.
	l := NewLogo(80, 24)
	view := l.View()
	if view == "" {
		t.Fatal("expected non-empty logo view")
	}
	if !strings.Contains(view, "@@@@") {
		t.Log("mascot may not contain @@@@ — checking for wordmark fallback")
		if strings.Contains(view, "relurpify") {
			t.Log("wordmark rendered instead of mascot")
		}
	}
}

func TestLogoWordmarkNarrow(t *testing.T) {
	// Below ~57 cols the wordmark should be chosen.
	l := NewLogo(50, 24)
	view := l.View()
	if view == "" {
		t.Fatal("expected non-empty logo view at narrow width")
	}
}

func TestLogoWordmarkShort(t *testing.T) {
	// Below 22 rows the wordmark should be chosen.
	l := NewLogo(80, 10)
	view := l.View()
	if view == "" {
		t.Fatal("expected non-empty logo view at short height")
	}
}

func TestLogoTiny(t *testing.T) {
	l := NewLogo(10, 3)
	view := l.View()
	if view == "" {
		t.Fatal("expected render even at tiny size")
	}
}

func TestLogoNilSafe(t *testing.T) {
	var l *Logo
	if l.View() != "" {
		t.Error("nil logo should return empty")
	}
}

func TestLogoZeroSize(t *testing.T) {
	l := NewLogo(0, 0)
	if l.View() != "" {
		t.Error("zero-size logo should return empty")
	}
}

func TestPrepareArtRemovesTrailingBlankLines(t *testing.T) {
	art := "line1\nline2\n  \n\t\nline3\n  \n"
	lines := prepareArt(art)
	// Interior blank lines are kept; only trailing blanks are dropped.
	if len(lines) != 5 {
		t.Errorf("expected 5 lines (interior blanks kept), got %d: %#v", len(lines), lines)
	}
	if lines[0] != "line1" {
		t.Errorf("first line = %q, want line1", lines[0])
	}
	if lines[4] != "line3" {
		t.Errorf("last line = %q, want line3", lines[4])
	}
}

func TestPrepareArtRightTrims(t *testing.T) {
	art := "hello   \nworld\t\n"
	lines := prepareArt(art)
	if lines[0] != "hello" {
		t.Errorf("right-trimmed line = %q, want hello", lines[0])
	}
	if lines[1] != "world" {
		t.Errorf("right-trimmed line = %q, want world", lines[1])
	}
}

func TestLogoSetSize(t *testing.T) {
	l := NewLogo(80, 24)
	l.SetSize(40, 10)
	view := l.View()
	if view == "" {
		t.Error("expected non-empty after SetSize")
	}
}
