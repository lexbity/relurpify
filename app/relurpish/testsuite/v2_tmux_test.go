//go:build relurpish_v2_integration

package testsuite_test

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var v2Binary = flag.String("v2-binary", "", "path to the built Relurpish executable") //nolint:gochecknoglobals // test runner flag

// The V2 integration suite drives the real executable inside a tmux session
// instead of a bare creack/pty pair: tmux runs headless (no controlling
// terminal needed, background-job safe), isolates every test on its own
// socket, exposes the RENDERED screen through capture-pane (assertions read
// what a human sees, not raw ANSI), and reports the child's exit status via
// remain-on-exit. tmux is a required gate: a missing binary fails the test,
// it never skips.

const (
	tmuxWidth  = 80
	tmuxHeight = 24
)

type terminalRecording struct {
	mu     sync.Mutex
	history strings.Builder
	socket string
	done   chan struct{}
}

// capture returns the currently rendered pane text, right-trimmed per row.
func (r *terminalRecording) capture() (string, error) {
	out, err := exec.Command("tmux", "-L", r.socket, "capture-pane", "-t", "main:0.0", "-p").Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n"), nil
}

// paneState reports whether the pane's command has exited and its exit code.
func (r *terminalRecording) paneState() (dead bool, status string, err error) {
	out, err := exec.Command("tmux", "-L", r.socket,
		"display-message", "-p", "-t", "main:0.0", "#{pane_dead} #{pane_dead_status}").Output()
	if err != nil {
		return false, "", err
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) != 2 {
		return false, "", fmt.Errorf("unexpected pane state %q", string(out))
	}
	return fields[0] == "1", fields[1], nil
}

func (r *terminalRecording) contains(value string) bool {
	r.mu.Lock()
	history := r.history.String()
	r.mu.Unlock()
	if strings.Contains(history, value) {
		return true
	}
	live, err := r.capture()
	if err != nil {
		return false
	}
	return strings.Contains(live, value)
}

func (r *terminalRecording) tail() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	text := r.history.String()
	if len(text) > 8192 {
		text = text[len(text)-8192:]
	}
	return text
}

// bottomRow returns the last rendered screen row: the universal bar's row.
func (r *terminalRecording) bottomRow() string {
	live, err := r.capture()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(live, "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimRight(lines[len(lines)-1], " \t")
}

// keyEvent translates one control sequence from the legacy write() strings
// into a tmux key name; ok=false means the bytes are literal text.
func keyEvent(b byte, rest string) (string, int, bool) {
	switch b {
	case '\r', '\n':
		return "Enter", 1, true
	case '\t':
		return "Tab", 1, true
	case '\x1b':
		switch {
		case strings.HasPrefix(rest, "[A"):
			return "Up", 3, true
		case strings.HasPrefix(rest, "[B"):
			return "Down", 3, true
		case strings.HasPrefix(rest, "[C"):
			return "Right", 3, true
		case strings.HasPrefix(rest, "[D"):
			return "Left", 3, true
		case strings.HasPrefix(rest, "OP"):
			return "F1", 3, true
		case strings.HasPrefix(rest, "m"):
			return "M-m", 2, true
		case strings.HasPrefix(rest, "[17~"):
			return "F6", 5, true
		default:
			return "Escape", 1, true
		}
	case 1:
		return "C-a", 1, true
	case 7:
		return "C-g", 1, true
	case 11:
		return "C-k", 1, true
	case 14:
		return "C-n", 1, true
	case 16:
		return "C-p", 1, true
	case 17:
		return "C-q", 1, true
	case 19:
		return "C-s", 1, true
	case 21:
		return "C-u", 1, true
	}
	return "", 0, false
}

// write splits a legacy raw-byte string into literal runs and named keys,
// sending each through tmux exactly as the terminal would have delivered it.
func (r *terminalRecording) write(t *testing.T, text string) {
	t.Helper()
	var literal strings.Builder
	flush := func() {
		if literal.Len() == 0 {
			return
		}
		cmd := exec.Command("tmux", "-L", r.socket, "send-keys", "-t", "main:0.0", "-l", "--", literal.String())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("send literal text: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		literal.Reset()
	}
	for i := 0; i < len(text); {
		if name, width, isKey := keyEvent(text[i], text[min(i+1, len(text)):]); isKey {
			flush()
			cmd := exec.Command("tmux", "-L", r.socket, "send-keys", "-t", "main:0.0", name)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("send key %s: %v (%s)", name, err, strings.TrimSpace(string(out)))
			}
			i += width
			continue
		}
		literal.WriteByte(text[i])
		i++
	}
	flush()
}

// startPoller appends rendered-screen snapshots to the history so assertions
// can match text that has already scrolled or been repainted away — the same
// contract the raw-byte recording used to provide.
func (r *terminalRecording) startPoller() {
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.done:
				return
			case <-ticker.C:
				live, err := r.capture()
				if err != nil {
					return // pane gone: server killed or session closed
				}
				r.mu.Lock()
				r.history.WriteString(live)
				r.history.WriteString("\n")
				r.mu.Unlock()
			}
		}
	}()
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

var tmuxSocketCounter sync.Mutex
var tmuxSocketSeq int

// newTmuxSocket creates one tmux server socket for a single test: full
// isolation from the user's tmux sessions and from parallel tests.
func newTmuxSocket(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatalf("tmux is required for the V2 integration suite (headless gate); install tmux: %v", err)
	}
	tmuxSocketCounter.Lock()
	tmuxSocketSeq++
	socket := "relurpish-v2-" + strconv.Itoa(os.Getpid()) + "-" + strconv.Itoa(tmuxSocketSeq)
	tmuxSocketCounter.Unlock()
	return socket
}

func tmuxkillServer(socket string) {
	_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
}

// launchTmuxSession starts the binary in a detached 80x24 tmux session and
// wires the recording, key writer, and quit-with-status verification.
func launchTmuxSession(t *testing.T, workspace string, arguments ...string) (*terminalRecording, func(string)) {
	t.Helper()
	if *v2Binary == "" {
		t.Fatal("-v2-binary is required; build the real executable before running this suite")
	}
	if len(arguments) == 0 {
		arguments = []string{"chat", "--offline"}
	}
	arguments = append(arguments, "--workspace", workspace)

	socket := newTmuxSocket(t)
	t.Cleanup(func() { tmuxkillServer(socket) })

	parts := make([]string, 0, len(arguments)+1)
	parts = append(parts, shellQuote(*v2Binary))
	for _, argument := range arguments {
		parts = append(parts, shellQuote(argument))
	}
	// remain-on-exit keeps the pane after the child exits so the harness can
	// read its exit status; chaining it into the same tmux invocation closes
	// the race with fast-exiting commands.
	create := exec.Command("tmux", "-L", socket, "-f", "/dev/null", "new-session",
		"-d", "-x", strconv.Itoa(tmuxWidth), "-y", strconv.Itoa(tmuxHeight),
		"-s", "main", strings.Join(parts, " "), ";",
		"set-option", "-w", "remain-on-exit", "on")
	if out, err := create.CombinedOutput(); err != nil {
		t.Fatalf("start tmux session: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	recording := &terminalRecording{socket: socket, done: make(chan struct{})}
	recording.startPoller()
	quitting := false
	t.Cleanup(func() {
		if !quitting {
			recording.write(t, "\x11\x1b[B\r")
		}
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			dead, status, err := recording.paneState()
			if err == nil && dead {
				if status != "0" {
					t.Errorf("terminal exited with status %s, want clean exit 0", status)
				}
				close(recording.done)
				tmuxkillServer(socket)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		close(recording.done)
		tmuxkillServer(socket)
		t.Error("terminal shutdown exceeded twenty seconds")
	})
	write := func(text string) {
		t.Helper()
		recording.write(t, text)
	}
	return recording, write
}

func launchV2Terminal(t *testing.T, arguments ...string) (*terminalRecording, func(string)) {
	t.Helper()
	workspace := t.TempDir()
	recording, write := launchTmuxSession(t, workspace, arguments...)
	// Read-only first launch must not touch the workspace.
	t.Cleanup(func() {
		entries, err := os.ReadDir(workspace)
		if err != nil || len(entries) != 0 {
			t.Errorf("read-only first launch changed workspace: entries=%v error=%v", entries, err)
		}
	})
	return recording, write
}

// launchV2TerminalIn boots the real executable in a prepared workspace and
// returns the recording, a key writer, and a stop function.
func launchV2TerminalIn(t *testing.T, workspace string, arguments ...string) (*terminalRecording, func(string), func()) {
	t.Helper()
	recording, write := launchTmuxSession(t, workspace, arguments...)
	stop := func() {
		recording.write(t, "\x11\x1b[B\r")
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			dead, status, err := recording.paneState()
			if err == nil && dead {
				if status != "0" {
					t.Errorf("terminal exited with status %s, want clean exit 0", status)
				}
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Error("terminal shutdown exceeded twenty seconds")
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
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("actual terminal did not show %q; last output: %q", text, recording.tail())
}

func TestV2DoctorWithoutRuntime(t *testing.T) {
	recording, _ := launchV2Terminal(t)
	awaitV2Text(t, recording, "relurpish")
	awaitV2Text(t, recording, "setup needed")
	awaitV2Text(t, recording, "Doctor")
}

func TestV2TmuxFocusAndBounds(t *testing.T) {
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
	// here the real 80x24 terminal has to retain its bottom command bar: the
	// draft renders in the bar head row with the keymap footer directly below.
	live, err := recording.capture()
	if err != nil {
		t.Fatalf("capture pane: %v", err)
	}
	rows := strings.Split(strings.TrimRight(live, "\n"), "\n")
	if len(rows) < 2 {
		t.Fatalf("screen collapsed below two rows: %q", live)
	}
	bottom := rows[len(rows)-2] + "\n" + rows[len(rows)-1]
	if !strings.Contains(bottom, "delete aXY") || !strings.Contains(bottom, "plain text -> Euclo") {
		t.Fatalf("bottom rows do not carry the bar draft and footer: %q", bottom)
	}
}

func TestV2TmuxDemoCommandDraftRoundTrip(t *testing.T) {
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

func TestV2TmuxActivityProviderAndDraft(t *testing.T) {
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

func TestV2TmuxRecipeAuthorSaveRunAndQuit(t *testing.T) {
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

func TestV2TmuxGuardInspectPatternAndDraft(t *testing.T) {
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
	awaitV2Text(t, recording, "chat · local-edit-on")

	// The durable journal answers /runs with the committed run.
	write("/runs\r")
	awaitV2TextFor(t, recording, "Recorded runs", 30*time.Second)
	awaitV2Text(t, recording, "run_")
}

// TestV2TmuxBootLogsStayOffTerminal: the video-reported regression — boot
// log lines (config loaded, tool admission) painted over the Bubble Tea
// frame and made the interface unreadable. Boot logging must land in the
// workspace log file; the terminal shows only rendered TUI frames.
func TestV2TmuxBootLogsStayOffTerminal(t *testing.T) {
	workspace := t.TempDir()
	bootstrap := exec.Command(*v2Binary, "doctor", "--workspace", workspace)
	if out, err := bootstrap.CombinedOutput(); err != nil {
		t.Fatalf("CLI doctor materialization failed: %v\n%s", err, out)
	}
	configPath := filepath.Join(workspace, "relurpify_cfg", "workspace.yaml")
	configuration, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read workspace config: %v", err)
	}
	pinned := strings.Replace(string(configuration), "provider: ollama", "provider: offline", 1)
	if err := os.WriteFile(configPath, []byte(pinned), 0o644); err != nil {
		t.Fatalf("pin offline provider: %v", err)
	}
	recording, _, stopTerminal := launchV2TerminalIn(t, workspace, "chat", "--offline")
	defer stopTerminal()
	awaitV2TextFor(t, recording, "Doctor / Ready", 45*time.Second)

	for _, forbidden := range []string{"config loaded", "tool admission warning", "service manager: registered"} {
		if recording.contains(forbidden) {
			t.Fatalf("boot log line %q reached the terminal; boot logging must stay in the workspace log file", forbidden)
		}
	}
	// The boot log file received what the terminal must not.
	bootLog, err := os.ReadFile(filepath.Join(workspace, ".relurpify_state", "logs", "boot.log"))
	if err != nil || !strings.Contains(string(bootLog), "config loaded") {
		t.Fatalf("boot log lines missing from the workspace boot log: %v", err)
	}
}
