package workspace

import (
	"path/filepath"
	"testing"
)

func TestNew_EmptyRoot(t *testing.T) {
	_, err := New("")
	if err == nil {
		t.Fatal("expected error for empty root")
	}
}

func TestNew_NormalizesPath(t *testing.T) {
	id, err := New("/tmp/../tmp")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean("/tmp")
	if id.Root != want {
		t.Errorf("Root = %q, want %q", id.Root, want)
	}
}

func TestNew_MakesAbsolute(t *testing.T) {
	// Relative path should be resolved to absolute.
	id, err := New(".")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(id.Root) {
		t.Errorf("Root = %q, want absolute path", id.Root)
	}
}

func TestIdentity_StateDir(t *testing.T) {
	id := Identity{Root: "/workspace"}
	want := "/workspace/.relurpify_state"
	got := id.StateDir()
	if got != want {
		t.Errorf("StateDir() = %q, want %q", got, want)
	}
}

func TestIdentity_LogDir(t *testing.T) {
	id := Identity{Root: "/workspace"}
	want := "/workspace/.relurpify_state/logs"
	got := id.LogDir()
	if got != want {
		t.Errorf("LogDir() = %q, want %q", got, want)
	}
}

func TestIdentity_LogPath(t *testing.T) {
	id := Identity{Root: "/workspace"}
	want := "/workspace/.relurpify_state/logs/agent.log"
	got := id.LogPath("agent.log")
	if got != want {
		t.Errorf("LogPath() = %q, want %q", got, want)
	}
}

func TestIdentity_TelemetryDir(t *testing.T) {
	id := Identity{Root: "/workspace"}
	want := "/workspace/.relurpify_state/telemetry"
	got := id.TelemetryDir()
	if got != want {
		t.Errorf("TelemetryDir() = %q, want %q", got, want)
	}
}

func TestIdentity_TelemetryPath(t *testing.T) {
	id := Identity{Root: "/workspace"}
	want := "/workspace/.relurpify_state/telemetry/events.jsonl"
	got := id.TelemetryPath("events.jsonl")
	if got != want {
		t.Errorf("TelemetryPath() = %q, want %q", got, want)
	}
}

func TestIdentity_EventsFile(t *testing.T) {
	id := Identity{Root: "/workspace"}
	want := "/workspace/.relurpify_state/events.db"
	got := id.EventsFile()
	if got != want {
		t.Errorf("EventsFile() = %q, want %q", got, want)
	}
}

func TestIdentity_MemoryDir(t *testing.T) {
	id := Identity{Root: "/workspace"}
	want := "/workspace/.relurpify_state/memory"
	got := id.MemoryDir()
	if got != want {
		t.Errorf("MemoryDir() = %q, want %q", got, want)
	}
}

func TestStateDir_PackageFunction(t *testing.T) {
	want := "/workspace/.relurpify_state"
	got := StateDir("/workspace")
	if got != want {
		t.Errorf("StateDir() = %q, want %q", got, want)
	}
}

func TestStateDirName_Constant(t *testing.T) {
	if StateDirName != ".relurpify_state" {
		t.Errorf("StateDirName = %q, want %q", StateDirName, ".relurpify_state")
	}
}

func TestIdentity_StateDir_EmptyRoot(t *testing.T) {
	id := Identity{Root: ""}
	got := id.StateDir()
	if got == "" {
		t.Error("StateDir() with empty root should be a relative path, not empty")
	}
}

func TestNew_InvalidPathSymlinkLoop(t *testing.T) {
	_, err := New("/dev/null/../loop")
	if err != nil {
		t.Logf("expected possible error for weird path: %v", err)
	}
}
