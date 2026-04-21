package ayenitd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	archaeobkc "codeburg.org/lexbit/relurpify/archaeo/bkc"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"github.com/stretchr/testify/require"
)

func TestGitWatcherServiceHelpersAndStart(t *testing.T) {
	t.Run("nil workspace is no-op", func(t *testing.T) {
		require.NoError(t, (&GitWatcherService{}).Start(context.Background()))
		require.NoError(t, (&GitWatcherService{}).Stop())
	})

	t.Run("runGit policy and helper paths", func(t *testing.T) {
		svc := &GitWatcherService{
			WorkspaceRoot: t.TempDir(),
			RunGit: func(ctx context.Context, root string, args ...string) (string, error) {
				if len(args) > 0 && args[0] == "rev-parse" {
					return "abc123\n", nil
				}
				return "file-one.go\n\n file-two.go \n", nil
			},
			Policy: sandbox.CommandPolicyFunc(func(context.Context, sandbox.CommandRequest) error {
				return errors.New("denied")
			}),
		}
		_, err := svc.runGit(context.Background(), "status")
		require.Error(t, err)

		svc.Policy = nil
		rev, err := svc.currentRevision(context.Background())
		require.NoError(t, err)
		require.Equal(t, "abc123", rev)

		paths, err := svc.affectedPaths(context.Background(), "deadbeef")
		require.NoError(t, err)
		require.Equal(t, []string{"file-one.go", "file-two.go"}, paths)
	})

	t.Run("start emits revision changed", func(t *testing.T) {
		bus := &archaeobkc.EventBus{}
		events, unsubscribe := bus.Subscribe(1)
		defer unsubscribe()

		svc := &GitWatcherService{
			WorkspaceRoot: t.TempDir(),
			EventBus:      bus,
			PollInterval:  time.Millisecond,
			LastRevision:  "old-rev",
		}
		var mu sync.Mutex
		calls := 0
		svc.RunGit = func(ctx context.Context, root string, args ...string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls == 1 {
				return "new-rev\n", nil
			}
			return "changed.go\n", nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- svc.Start(ctx)
		}()

		select {
		case event := <-events:
			require.Equal(t, archaeobkc.EventCodeRevisionChanged, event.Kind)
			payload, ok := event.Payload.(archaeobkc.CodeRevisionChangedPayload)
			require.True(t, ok)
			require.Equal(t, "new-rev", payload.NewRevision)
			require.Equal(t, []string{"changed.go"}, payload.AffectedPaths)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for code revision event")
		}

		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for watcher shutdown")
		}

		require.NoError(t, svc.Stop())
	})

	t.Run("default exec path", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git not available")
		}

		dir := t.TempDir()
		require.NoError(t, exec.Command("git", "init", dir).Run())
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("one"), 0o644))
		require.NoError(t, exec.Command("git", "-C", dir, "add", "tracked.txt").Run())
		require.NoError(t, exec.Command("git", "-C", dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "init").Run())
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("two"), 0o644))
		require.NoError(t, exec.Command("git", "-C", dir, "add", "tracked.txt").Run())
		require.NoError(t, exec.Command("git", "-C", dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "update").Run())

		svc := &GitWatcherService{WorkspaceRoot: dir}
		rev, err := svc.currentRevision(context.Background())
		require.NoError(t, err)
		require.NotEmpty(t, rev)

		paths, err := svc.affectedPaths(context.Background(), rev)
		require.NoError(t, err)
		require.Contains(t, paths, "tracked.txt")
	})
}
