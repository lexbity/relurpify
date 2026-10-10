package ayenitd

// refresh_submitter_test.go proves the FR-23 scheduler consumer: the
// runner-side scheduled submission re-issues knowledge.refresh through the
// spool on its cadence. The scheduler runs a job immediately on Start, so a
// short interval suffices to observe the submission without waiting.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRefreshSubmitter_SubmitsKnowledgeRefresh(t *testing.T) {
	stateDir := t.TempDir()
	client, err := NewSpoolClient(stateDir, "relurpify-runner", t.TempDir())
	require.NoError(t, err)

	submitter := newRefreshSubmitter(50*time.Millisecond, []string{"knowledge"}, client)
	require.NoError(t, submitter.Start(context.Background()))
	defer func() { _ = submitter.Stop() }()

	// The scheduled job runs on Start: a knowledge.refresh spool file lands
	// in pending/ without any app involvement.
	var files []string
	require.Eventually(t, func() bool {
		entries, err := os.ReadDir(filepath.Join(stateDir, "jobs", "spool", "pending"))
		if err != nil {
			return false
		}
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
				files = append(files, e.Name())
			}
		}
		return len(files) > 0
	}, 5*time.Second, 20*time.Millisecond, "the scheduled submission must spool a knowledge.refresh job")

	// The spooled file is a valid knowledge.refresh submission for the
	// client's workspace.
	data, err := os.ReadFile(filepath.Join(stateDir, "jobs", "spool", "pending", files[0]))
	require.NoError(t, err)
	var f spoolFile
	require.NoError(t, json.Unmarshal(data, &f))
	require.Equal(t, "knowledge.refresh", f.Spec.Kind)
	require.NotEmpty(t, f.CorrelateID)
	require.Equal(t, "relurpify-runner", f.Producer)
	require.Equal(t, "knowledge", f.Spec.Queue)
	require.Contains(t, f.Spec.Payload, "workspace_root")
}

func TestRefreshSubmitter_StopIsIdempotent(t *testing.T) {
	client, err := NewSpoolClient(t.TempDir(), "relurpify-runner", t.TempDir())
	require.NoError(t, err)
	submitter := newRefreshSubmitter(time.Hour, []string{"knowledge"}, client)
	require.NoError(t, submitter.Stop(), "Stop before Start is a no-op")
}
