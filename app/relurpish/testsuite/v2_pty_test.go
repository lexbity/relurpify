//go:build relurpish_v2_integration

package testsuite_test

import (
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

var v2Binary = flag.String("v2-binary", "", "path to the built Relurpish executable") //nolint:gochecknoglobals // test runner flag

type terminalRecording struct {
	mu   sync.Mutex
	text strings.Builder
}

func (r *terminalRecording) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = r.text.Write(data)
	return len(data), nil
}

func (r *terminalRecording) contains(value string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Contains(r.text.String(), value)
}

func (r *terminalRecording) tail() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	text := r.text.String()
	if len(text) > 8192 {
		text = text[len(text)-8192:]
	}
	return text
}

func launchV2Terminal(t *testing.T, arguments ...string) (*terminalRecording, func(string)) {
	t.Helper()
	if *v2Binary == "" {
		t.Fatal("-v2-binary is required; build the real executable before running this suite")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	workspace := t.TempDir()
	if len(arguments) == 0 {
		arguments = []string{"chat", "--offline"}
	}
	arguments = append(arguments, "--workspace", workspace)
	cmd := exec.CommandContext(ctx, *v2Binary, arguments...)
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		cancel()
		t.Fatalf("start actual PTY: %v", err)
	}
	recording := &terminalRecording{}
	drained := make(chan struct{})
	go func() { _, _ = io.Copy(recording, terminal); close(drained) }()
	t.Cleanup(func() {
		_, _ = terminal.Write([]byte("\x11\x1b[B\r"))
		finished := make(chan error, 1)
		go func() { finished <- cmd.Wait() }()
		select {
		case err := <-finished:
			if err != nil {
				t.Errorf("terminal did not exit cleanly: %v", err)
			}
		case <-time.After(5 * time.Second):
			cancel()
			<-finished
			t.Error("terminal shutdown exceeded five seconds")
		}
		cancel()
		_ = terminal.Close()
		<-drained
		entries, err := os.ReadDir(workspace)
		if err != nil || len(entries) != 0 {
			t.Errorf("read-only first launch changed workspace: entries=%v error=%v", entries, err)
		}
	})
	write := func(text string) {
		t.Helper()
		if _, err := terminal.Write([]byte(text)); err != nil {
			t.Fatalf("write terminal: %v", err)
		}
	}
	return recording, write
}

// launchV2TerminalIn boots the real executable in a prepared workspace and
// returns the recording, a key writer, and a stop function. The stop function
// quits via the quit review and waits for a clean exit.
func launchV2TerminalIn(t *testing.T, workspace string, arguments ...string) (*terminalRecording, func(string), func()) {
	t.Helper()
	if *v2Binary == "" {
		t.Fatal("-v2-binary is required; build the real executable before running this suite")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	arguments = append(arguments, "--workspace", workspace)
	cmd := exec.CommandContext(ctx, *v2Binary, arguments...)
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		cancel()
		t.Fatalf("start actual PTY: %v", err)
	}
	recording := &terminalRecording{}
	drained := make(chan struct{})
	go func() { _, _ = io.Copy(recording, terminal); close(drained) }()
	stop := func() {
		_, _ = terminal.Write([]byte("\x11\x1b[B\r"))
		finished := make(chan error, 1)
		go func() { finished <- cmd.Wait() }()
		select {
		case err := <-finished:
			if err != nil {
				t.Errorf("terminal did not exit cleanly: %v", err)
			}
		case <-time.After(20 * time.Second):
			cancel()
			<-finished
			t.Error("terminal shutdown exceeded twenty seconds")
		}
		cancel()
		_ = terminal.Close()
		<-drained
	}
	write := func(text string) {
		t.Helper()
		if _, err := terminal.Write([]byte(text)); err != nil {
			t.Fatalf("write terminal: %v", err)
		}
	}
	return recording, write, stop
}

func awaitV2Text(t *testing.T, recording *terminalRecording, text string) {
	t.Helper()
	awaitV2TextFor(t, recording, text, 10*time.Second)
}

func awaitV2TextFor(t *testing.T, recording *terminalRecording, text string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if recording.contains(text) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("actual terminal did not show %q; last output: %q", text, recording.tail())
}

func TestV2DoctorWithoutRuntime(t *testing.T) {
	recording, _ := launchV2Terminal(t)
	awaitV2Text(t, recording, "relurpish")
	awaitV2Text(t, recording, "setup needed")
	awaitV2Text(t, recording, "Doctor")
}

func TestV2PTYFocusAndBounds(t *testing.T) {
	recording, write := launchV2Terminal(t)
	awaitV2Text(t, recording, "setup needed")
	write("delete a")
	awaitV2Text(t, recording, "delete a")
	write("\r")
	// A blocked submission must retain the old draft; the next inserted character
	// distinguishes preservation from merely observing an earlier rendered frame.
	time.Sleep(100 * time.Millisecond)
	write("X")
	awaitV2Text(t, recording, "delete aX")
	write("\x1bOP") // F1
	awaitV2Text(t, recording, "Commands")
	write("\x1b")
	time.Sleep(100 * time.Millisecond)
	write("Y")
	awaitV2Text(t, recording, "delete aXY")
	// The renderer's own cell bounds are exhaustively checked by the model suite;
	// here the real 80x24 PTY also has to retain its bottom-row command bar.
	awaitV2Text(t, recording, "\x1b[24;")
}

func TestV2PTYDemoCommandDraftRoundTrip(t *testing.T) {
	recording, write := launchV2Terminal(t, "--demo", "quiet", "--inference-provider", "deliberately-invalid")
	awaitV2Text(t, recording, "DEMO")
	write("keep this request")
	awaitV2Text(t, recording, "keep this request")
	write("\x10") // Ctrl+P borrows the bar for a command.
	write("settings\r")
	awaitV2Text(t, recording, "Settings")
	write("\x1b[17~") // F6 returns to the preserved request draft.
	write("!")
	awaitV2Text(t, recording, "keep this request!")
}

func TestV2PTYActivityProviderAndDraft(t *testing.T) {
	recording, write := launchV2Terminal(t, "--demo", "quiet", "--inference-provider", "deliberately-invalid")
	awaitV2Text(t, recording, "DEMO")
	write("retained request")
	write("\x10activity calls\r")
	awaitV2Text(t, recording, "Activity / calls")
	write("\r")
	awaitV2Text(t, recording, "DEMO: simulated telemetry fixture")
	write("\x1b")
	time.Sleep(100 * time.Millisecond) // Distinguish two Escape keys from a terminal chord.
	write("\x1b")
	time.Sleep(100 * time.Millisecond)
	write("\x10model-provider\r")
	awaitV2Text(t, recording, "Settings / Provider")
	awaitV2Text(t, recording, "demo-small")
	write("\x1b[B\x1b[B\r") // Select the model and explicitly enter its editor.
	awaitV2Text(t, recording, "Edit Model")
	write("\x01\x0bdemo-coder\r") // Emacs home, kill-to-end, replacement, stage.
	awaitV2Text(t, recording, "demo-coder")
	write("s")
	awaitV2Text(t, recording, "Review save")
	write("\r")
	awaitV2Text(t, recording, "active scenario did not change")
	write("a")
	awaitV2Text(t, recording, "Review activate")
	write("\r")
	awaitV2Text(t, recording, "activated in scenario memory")
	write("\x1b[17~!")
	awaitV2Text(t, recording, "retained request!")
}

func TestV2PTYRecipeAuthorSaveRunAndQuit(t *testing.T) {
	recording, write := launchV2Terminal(t, "--demo", "quiet", "--inference-provider", "deliberately-invalid")
	awaitV2Text(t, recording, "DEMO")
	write("preserve this draft")
	write("\x10recipe new recipes/new_flow.erpe\r")
	awaitV2Text(t, recording, "New local draft")
	write("\x10recipe save\r")
	awaitV2Text(t, recording, "Review saved source change")
	write("\r")
	awaitV2Text(t, recording, "DEMO: source saved in memory @1")
	write("\x10recipe run recipes/new_flow.erpe\r")
	awaitV2Text(t, recording, "Trigger saved file")
	write("\x13") // Ctrl+S deliberately confirms the separate task-input field.
	awaitV2Text(t, recording, "Execution / new_flow")
	awaitV2Text(t, recording, "DEMO · completed")
	write("\x11") // The immutable quit chord opens a review, even from a field.
	awaitV2Text(t, recording, "Leave relurpish?")
	write("\x1b")
	time.Sleep(100 * time.Millisecond)
	write("\x1b[17~!")
	awaitV2Text(t, recording, "preserve this draft!")
}

func TestV2PTYGuardInspectPatternAndDraft(t *testing.T) {
	recording, write := launchV2Terminal(t, "--demo", "quiet", "--inference-provider", "deliberately-invalid")
	awaitV2Text(t, recording, "DEMO")
	write("retain guard request")
	write("\x10securityguard\r")
	awaitV2Text(t, recording, "Settings / Guard / shell")
	awaitV2Text(t, recording, "destructive-root")
	write("\r")
	awaitV2Text(t, recording, "Source revision")
	write("t")
	awaitV2Text(t, recording, "Matcher test only; command not executed")
	write("rm -rf /\r")
	awaitV2Text(t, recording, "Matches selected pattern: block")
	write("\t")
	awaitV2Text(t, recording, "Settings / Guard / ingestion")
	write("t")
	awaitV2Text(t, recording, "until G2 is implemented")
	write("\x1b[17~!")
	awaitV2Text(t, recording, "retain guard request!")
}

// TestV2RuntimeOwnsRunAfterNavigation: a submitted run is admitted and
// journaled by the runtime's dispatcher. Navigating canvases mid-turn never
// disturbs it, and /runs reads the committed outcome from the durable journal
// (P02). The offline backend keeps the turn deterministic; no model service
// is contacted.
func TestV2RuntimeOwnsRunAfterNavigation(t *testing.T) {
	// The documented flow materializes the configuration with the CLI doctor
	// (TUI initialization on a missing manifest is an explicit P05 deferral).
	workspace := t.TempDir()
	bootstrap := exec.Command(*v2Binary, "doctor", "--workspace", workspace)
	if out, err := bootstrap.CombinedOutput(); err != nil {
		t.Fatalf("CLI doctor materialization failed: %v\n%s", err, out)
	}
	// Pin the offline provider so runtime composition is deterministic and
	// fast: the sandbox and model lanes still run their real paths.
	configPath := filepath.Join(workspace, "relurpify_cfg", "workspace.yaml")
	configuration, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read workspace config: %v", err)
	}
	pinned := strings.Replace(string(configuration), "provider: ollama", "provider: offline", 1)
	if pinned == string(configuration) {
		t.Fatalf("workspace config did not name the ollama provider:\n%s", configuration)
	}
	if err := os.WriteFile(configPath, []byte(pinned), 0o644); err != nil {
		t.Fatalf("pin offline provider: %v", err)
	}
	recording, write, stopTerminal := launchV2TerminalIn(t, workspace, "chat", "--offline", "--hitl-mode", "permissive")
	defer stopTerminal()
	// The pane verdict is the readiness signal: the host status row can be
	// clipped when a canvas body exceeds its row budget (known layout bug,
	// tracked separately), so the test reads the verdict itself.
	awaitV2TextFor(t, recording, "Doctor / Ready", 45*time.Second)

	// Submit a real instruction through the dispatcher. A submission fired
	// before the host unlocked is kept as a draft (never dropped), so the
	// test resubmits until the turn is admitted.
	for attempt := 0; attempt < 6; attempt++ {
		write("Summarize the workspace layout in one line\r")
		deadline := time.Now().Add(3 * time.Second)
		admitted := false
		for time.Now().Before(deadline) {
			if recording.contains("working · /cancel") || recording.contains("kept as a draft while work is running") {
				admitted = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if admitted {
			break
		}
	}

	// Navigate away mid-turn and back: selection and drafts survive, and no
	// navigation error appears. The run's lifetime is the dispatcher's.
	write("/run\r")
	awaitV2Text(t, recording, "Execution")
	write("/chat\r")
	awaitV2Text(t, recording, "relurpish · euclo / chat")

	// The durable journal answers /runs with the committed run.
	write("/runs\r")
	awaitV2TextFor(t, recording, "Recorded runs", 30*time.Second)
	awaitV2Text(t, recording, "run_")
}
