package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// deadPID is beyond the system pid_max (4194304 on typical Linux), so
// syscall.Kill(deadPID, 0) reliably returns ESRCH → "no live owner".
const deadPID = 2147483647

// psLine renders a docker ps --format row (name\tlabels).
func psLine(name string, labels map[string]string) string {
	parts := ""
	for _, k := range []string{LabelManaged, LabelWorkspace, LabelPID, LabelCreated} {
		v := labels[k]
		if parts != "" {
			parts += ","
		}
		parts += k + "=" + v
	}
	return name + "\t" + parts
}

func reapWithContent(t *testing.T, content string, now time.Time, opts ReapOptions) ReapReport {
	t.Helper()
	fakeDockerEnv(t)
	t.Setenv("FAKE_DOCKER_PS_CONTENT", content)
	if opts.Now == nil {
		opts.Now = func() time.Time { return now }
	}
	report, err := ReapOrphans(context.Background(), opts)
	require.NoError(t, err)
	return report
}

func TestReapOrphans_LiveOwnerKept(t *testing.T) {
	now := time.Now()
	content := psLine("relurpify-keep", map[string]string{
		LabelManaged:   "true",
		LabelPID:       strconv.Itoa(os.Getpid()),
		LabelCreated:   strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10),
		LabelWorkspace: "ab12cd34",
	})
	report := reapWithContent(t, content, now, ReapOptions{})
	require.Equal(t, 1, report.Scanned)
	require.Equal(t, 0, report.Reaped, "a container owned by the live supervisor must be kept")
}

func TestReapOrphans_DeadOwnerPastGraceReaped(t *testing.T) {
	now := time.Now()
	content := psLine("relurpify-reap", map[string]string{
		LabelManaged:   "true",
		LabelPID:       strconv.FormatInt(deadPID, 10),
		LabelCreated:   strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10),
		LabelWorkspace: "ab12cd34",
	})
	report := reapWithContent(t, content, now, ReapOptions{})
	require.Equal(t, 1, report.Scanned)
	require.Equal(t, 1, report.Reaped)
	require.Equal(t, []string{"relurpify-reap"}, report.ReapedNames)
}

func TestReapOrphans_DeadOwnerWithinGraceKept(t *testing.T) {
	now := time.Now()
	content := psLine("relurpify-young", map[string]string{
		LabelManaged:   "true",
		LabelPID:       strconv.FormatInt(deadPID, 10),
		LabelCreated:   strconv.FormatInt(now.Add(-time.Minute).Unix(), 10),
		LabelWorkspace: "ab12cd34",
	})
	report := reapWithContent(t, content, now, ReapOptions{})
	require.Equal(t, 0, report.Reaped, "a fresh dead-owner container must be kept within the grace window")
}

func TestReapOrphans_MaxAgeReapsLiveOwner(t *testing.T) {
	now := time.Now()
	// Own pid, so the owner is alive; the absolute age cap still wins.
	content := psLine("relurpify-ancient", map[string]string{
		LabelManaged:   "true",
		LabelPID:       strconv.Itoa(os.Getpid()),
		LabelCreated:   strconv.FormatInt(now.Add(-25*time.Hour).Unix(), 10),
		LabelWorkspace: "ab12cd34",
	})
	report := reapWithContent(t, content, now, ReapOptions{})
	require.Equal(t, 1, report.Reaped, "the absolute age cap must reap regardless of owner liveness")
}

func TestReapOrphans_ForeignLiveOwnerKept(t *testing.T) {
	now := time.Now()
	// pid 1 (init) is alive on Linux; a concurrent instance's live container
	// must never be reaped (R-7).
	content := psLine("relurpify-foreign", map[string]string{
		LabelManaged:   "true",
		LabelPID:       "1",
		LabelCreated:   strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10),
		LabelWorkspace: "cd34ef56",
	})
	report := reapWithContent(t, content, now, ReapOptions{})
	require.Equal(t, 0, report.Reaped, "a container owned by another live process must be kept")
}

func TestReapOrphans_BudgetExhaustionDoesNotHang(t *testing.T) {
	now := time.Now()
	var content string
	for i := 0; i < 20; i++ {
		content += psLine(fmt.Sprintf("relurpify-budget-%02d", i), map[string]string{
			LabelManaged:   "true",
			LabelPID:       strconv.FormatInt(deadPID, 10),
			LabelCreated:   strconv.FormatInt(now.Add(-30*time.Minute).Unix(), 10),
			LabelWorkspace: "ab12cd34",
		}) + "\n"
	}
	// A sub-nanosecond budget expires before any rm runs: partial, no hang.
	report := reapWithContent(t, content, now, ReapOptions{Budget: time.Nanosecond})
	require.Equal(t, 20, report.Scanned)
	require.Equal(t, 0, report.Reaped, "budget exhaustion must yield a partial report, not a hang")
}

func TestReapOrphans_MissingCreatedTreatedAsOld(t *testing.T) {
	now := time.Now()
	content := "relurpify-nodate\trelurpify.managed=true,relurpify.pid=" + strconv.FormatInt(deadPID, 10)
	report := reapWithContent(t, content, now, ReapOptions{})
	require.Equal(t, 1, report.Reaped, "a managed container without a created stamp must be reclaimable")
}

func TestReapOrphans_DockerMissingReturnsError(t *testing.T) {
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty")) // no docker anywhere
	_, err := ReapOrphans(context.Background(), ReapOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestReapOrphans_DockerPSFailureReturnsError(t *testing.T) {
	fakeDockerEnv(t)
	t.Setenv("FAKE_DOCKER_PS_FAIL", "1")
	_, err := ReapOrphans(context.Background(), ReapOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "docker ps failed")
}
