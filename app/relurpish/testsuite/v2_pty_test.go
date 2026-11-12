//go:build relurpish_v2_integration

package testsuite_test

import (
	"context"
	"flag"
	"io"
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
	text:=r.text.String()
	if len(text)>8192 { text=text[len(text)-8192:] }
	return text
}

func launchV2Terminal(t *testing.T) (*terminalRecording, func(string)) {
	t.Helper()
	if *v2Binary == "" {
		t.Fatal("-v2-binary is required; build the real executable before running this suite")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	cmd := exec.CommandContext(ctx, *v2Binary, "chat", "--offline", "--workspace", t.TempDir())
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		cancel()
		t.Fatalf("start actual PTY: %v", err)
	}
	recording := &terminalRecording{}
	drained := make(chan struct{})
	go func() { _, _ = io.Copy(recording, terminal); close(drained) }()
	t.Cleanup(func() {
		_, _ = terminal.Write([]byte{3})
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
	})
	write := func(text string) {
		t.Helper()
		if _, err := terminal.Write([]byte(text)); err != nil {
			t.Fatalf("write terminal: %v", err)
		}
	}
	return recording, write
}

func awaitV2Text(t *testing.T, recording *terminalRecording, text string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if recording.contains(text) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("actual terminal did not show %q; last output: %q", text,recording.tail())
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
