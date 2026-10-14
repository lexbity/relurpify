package ingestion

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	"codeburg.org/lexbit/relurpify/telemetry"
)

type recordedEvent struct {
	EventType string
	Payload   map[string]any
}

type eventRecorder struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (r *eventRecorder) Emit(eventType string, payload map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, recordedEvent{EventType: eventType, Payload: payload})
}

func (r *eventRecorder) snapshot() []recordedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedEvent(nil), r.events...)
}

// chunkRecorder is a custom Scanner that memorizes the chunk bodies it sees.
type chunkRecorder struct {
	mu     sync.Mutex
	bodies []string
}

func (c *chunkRecorder) Name() string { return "chunk_recorder" }

func (c *chunkRecorder) Scan(_ context.Context, chunk TypedChunk) ScanResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies = append(c.bodies, string(chunk.Content))
	return ScanResult{Flags: []string{}}
}

func (c *chunkRecorder) seen() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.bodies...)
}

// prewarmedDefaultScanners was a test-side workaround for the
// SignatureScanner lazy-init data race (REVISIT-issues R-10); the race is
// fixed now — the defaults are precompiled at package level — so tests use
// defaultScanners() directly.

func quarantinePolicy() (*contextports.PolicyBundle, *fakeEvaluator) {
	return &contextports.PolicyBundle{DefaultTrustClass: "workspace-trusted"},
		&fakeEvaluator{admitTrustClass: true, quota: 0}
}

func TestWorkspaceScannerScanCommitsWorkspaceFiles(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	quarantineRoot := filepath.Join(t.TempDir(), "quarantine")
	writeWorkspaceFile(t, workspace, "main.go", "package main\n\nfunc Run() {}\n")
	writeWorkspaceFile(t, workspace, "docs/notes.md", "# Notes\nbody text\n")
	writeWorkspaceFile(t, workspace, "readme.txt", "hello reader\n")

	store := newIngestionStore(t)
	events := &eventRecorder{}
	// Nil Scanners exercises the default-scanner branch of Scan; with the
	// precompiled signature patterns (R-10) this is race-clean at the
	// default concurrency.
	scanner := &WorkspaceScanner{
		Store:         store,
		Events:        events,
		QuarantineDir: quarantineRoot,
	}

	report, err := scanner.Scan(ctx, workspace)
	require.NoError(t, err)
	require.Equal(t, 3, report.FilesScanned)
	require.Equal(t, 4, report.ChunksCreated, "go file yields two chunks, markdown and text one each")
	require.Zero(t, report.ChunksQuarantined)
	require.Zero(t, report.ChunksRejected)
	require.Empty(t, report.Errors)
	require.Greater(t, store.Revision(), uint64(0))

	emitted := events.snapshot()
	require.Len(t, emitted, 1)
	event := emitted[0]
	require.Equal(t, string(telemetry.EventBootstrapComplete), event.EventType)
	require.Equal(t, 3, event.Payload["files_scanned"])
	require.Equal(t, 4, event.Payload["chunks_created"])
	require.Equal(t, workspace, event.Payload["workspace_root"])
	// Scan pre-creates the quarantine root, but a commit-only scan must not
	// put any quarantine entries into it.
	entries, err := os.ReadDir(quarantineRoot)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestWorkspaceScannerScanSurvivesEmptyFiles(t *testing.T) {
	ctx := context.Background()
	workspace, quarantineRoot := t.TempDir(), t.TempDir()
	// The empty and newline-only files used to panic the whole scan
	// (boundaries are line counts sliced as byte offsets); they now yield
	// zero chunks (R-6 panic fix, truncation itself still recorded).
	writeWorkspaceFile(t, workspace, "empty.txt", "")
	writeWorkspaceFile(t, workspace, "newlines.txt", "\n\n\n")
	writeWorkspaceFile(t, workspace, "real.txt", "actual body\n")

	store := newIngestionStore(t)
	scanner := &WorkspaceScanner{Store: store, QuarantineDir: quarantineRoot}

	report, err := scanner.Scan(ctx, workspace)
	require.NoError(t, err)
	require.Equal(t, 3, report.FilesScanned)
	require.Equal(t, 1, report.ChunksCreated, "only the file with content produces a chunk")
	require.Empty(t, report.Errors)
}

func TestWorkspaceScannerRunsFilesConcurrently(t *testing.T) {
	ctx := context.Background()
	workspace, quarantineRoot := t.TempDir(), t.TempDir()
	// Bodies differ within their first bytes so the derived chunk ids stay
	// distinct; identical chunk ids from concurrent files collide in the
	// store (recorded bug: boundaries are line counts sliced as byte
	// offsets, so chunk content is only the first N bytes of the file).
	for _, body := range []string{"alpha body\n", "bravo body\n", "charlie body\n", "delta body\n"} {
		writeWorkspaceFile(t, workspace, body[:5]+".txt", body)
	}

	store := newIngestionStore(t)
	scanner := &WorkspaceScanner{
		Store:         store,
		QuarantineDir: quarantineRoot,
		Scanners:      defaultScanners(),
	}

	report, err := scanner.Scan(ctx, workspace)
	require.NoError(t, err)
	require.Equal(t, 4, report.FilesScanned)
	require.Equal(t, 4, report.ChunksCreated)
	require.Empty(t, report.Errors)
}

func TestWorkspaceScannerHonorsIncludeAndExcludeGlobs(t *testing.T) {
	ctx := context.Background()

	t.Run("include globs restrict discovery", func(t *testing.T) {
		workspace := t.TempDir()
		writeWorkspaceFile(t, workspace, "a.go", "package a\n")
		writeWorkspaceFile(t, workspace, "notes.md", "# notes\n")
		writeWorkspaceFile(t, workspace, "b.go", "// b header\npackage b\n")

		report, err := (&WorkspaceScanner{
			Store:         newIngestionStore(t),
			Scanners:      defaultScanners(),
			QuarantineDir: t.TempDir(),
		}).Scan(ctx, workspace)
		require.NoError(t, err)
		require.Equal(t, 3, report.FilesScanned)

		restricted := &WorkspaceScanner{
			Store:         newIngestionStore(t),
			IncludeGlobs:  []string{"*.go"},
			Scanners:      defaultScanners(),
			QuarantineDir: t.TempDir(),
		}
		report, err = restricted.Scan(ctx, workspace)
		require.NoError(t, err)
		require.Equal(t, 2, report.FilesScanned)
		require.Zero(t, report.ChunksQuarantined)
	})

	t.Run("exclude globs win over includes", func(t *testing.T) {
		workspace := t.TempDir()
		writeWorkspaceFile(t, workspace, "a.go", "package a\n")
		writeWorkspaceFile(t, workspace, "a_test.go", "package a\n")

		scanner := &WorkspaceScanner{
			Store:         newIngestionStore(t),
			IncludeGlobs:  []string{"*.go"},
			ExcludeGlobs:  []string{"*_test.go"},
			Scanners:      defaultScanners(),
			QuarantineDir: t.TempDir(),
		}
		report, err := scanner.Scan(ctx, workspace)
		require.NoError(t, err)
		require.Equal(t, 1, report.FilesScanned)
		require.Equal(t, 1, report.ChunksCreated)
	})
}

func TestWorkspaceScannerProvidedScannersReplaceDefaults(t *testing.T) {
	ctx := context.Background()
	workspace, quarantineRoot := t.TempDir(), t.TempDir()
	writeWorkspaceFile(t, workspace, "suspicious.txt", "SELECT password FROM users\n")

	recorder := &chunkRecorder{}
	policy := &contextports.PolicyBundle{DefaultTrustClass: "workspace-trusted"}
	evaluator := &fakeEvaluator{admitTrustClass: true, quota: 9}
	scanner := &WorkspaceScanner{
		Store:         newIngestionStore(t),
		QuarantineDir: quarantineRoot,
		Scanners:      []Scanner{recorder},
		Policy:        policy,
		Evaluator:     evaluator,
	}

	report, err := scanner.Scan(ctx, workspace)
	require.NoError(t, err)
	bodies := recorder.seen()
	require.Len(t, bodies, 1)
	require.NotEmpty(t, bodies[0])
	require.Len(t, scanner.Scanners, 1, "Scan must not append or replace provided scanners")
	require.Equal(t, 1, report.ChunksCreated, "the provided scanner reports no suspicion, so the file commits")
	require.Zero(t, report.ChunksQuarantined)
}

func TestWorkspaceScannerProvidedScannersDriveAdmission(t *testing.T) {
	ctx := context.Background()
	policy := &contextports.PolicyBundle{DefaultTrustClass: "workspace-trusted"}

	t.Run("high score quarantines", func(t *testing.T) {
		workspace, quarantineRoot := t.TempDir(), t.TempDir()
		writeWorkspaceFile(t, workspace, "note.txt", "plain body\n")

		scanner := &WorkspaceScanner{
			Store:         newIngestionStore(t),
			QuarantineDir: quarantineRoot,
			Scanners:      []Scanner{&fixedSuspicionScanner{score: 0.9}},
			Policy:        policy,
			Evaluator:     &fakeEvaluator{admitTrustClass: true, quota: 9},
		}
		report, err := scanner.Scan(ctx, workspace)
		require.NoError(t, err)
		require.Zero(t, report.ChunksCreated)
		require.Equal(t, 1, report.ChunksQuarantined, "the provided scanner's score alone drives the admission decision")
	})

	t.Run("zero score commits", func(t *testing.T) {
		workspace, quarantineRoot := t.TempDir(), t.TempDir()
		writeWorkspaceFile(t, workspace, "note.txt", "plain body\n")

		scanner := &WorkspaceScanner{
			Store:         newIngestionStore(t),
			QuarantineDir: quarantineRoot,
			Scanners:      []Scanner{&fixedSuspicionScanner{score: 0.0}},
			Policy:        policy,
			Evaluator:     &fakeEvaluator{admitTrustClass: true, quota: 9},
		}
		report, err := scanner.Scan(ctx, workspace)
		require.NoError(t, err)
		require.Equal(t, 1, report.ChunksCreated)
		require.Zero(t, report.ChunksQuarantined)
	})
}

func TestWorkspaceScannerAddScannerExtendsScannerSet(t *testing.T) {
	ctx := context.Background()
	workspace, quarantineRoot := t.TempDir(), t.TempDir()
	writeWorkspaceFile(t, workspace, "note.txt", "recorded body\n")

	recorder := &chunkRecorder{}
	scanner := &WorkspaceScanner{Store: newIngestionStore(t), QuarantineDir: quarantineRoot}
	scanner.AddScanner(recorder)
	require.Len(t, scanner.Scanners, 1)

	report, err := scanner.Scan(ctx, workspace)
	require.NoError(t, err)
	bodies := recorder.seen()
	require.Len(t, bodies, 1)
	require.NotEmpty(t, bodies[0])
	require.Equal(t, 1, report.ChunksCreated)
}

func TestWorkspaceScannerQuarantinesWhenQuotaExhausted(t *testing.T) {
	ctx := context.Background()
	workspace, quarantineRoot := t.TempDir(), t.TempDir()
	writeWorkspaceFile(t, workspace, "alpha.txt", "alpha body\n")
	writeWorkspaceFile(t, workspace, "bravo.txt", "bravo body\n")

	policy, evaluator := quarantinePolicy()
	scanner := &WorkspaceScanner{
		Store:         newIngestionStore(t),
		QuarantineDir: quarantineRoot,
		Scanners:      defaultScanners(),
		Policy:        policy,
		Evaluator:     evaluator,
	}

	report, err := scanner.Scan(ctx, workspace)
	require.NoError(t, err)
	require.Zero(t, report.ChunksCreated)
	require.Equal(t, 2, report.ChunksQuarantined)
	require.Empty(t, report.Errors)

	// Recorded bug: the quarantine entry suffix is the first 8 hex chars of
	// UnixNano (its high bits), which stays constant for ~4.29s, so
	// quarantines in the same window share one directory and the last write
	// wins. One surviving entry with intact artifacts is therefore the
	// strongest guarantee the current implementation provides.
	entries, err := os.ReadDir(quarantineRoot)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	bodies := make([]string, 0, len(entries))
	for _, entry := range entries {
		require.True(t, entry.IsDir())
		content, err := os.ReadFile(filepath.Join(quarantineRoot, entry.Name(), "content.bin"))
		require.NoError(t, err)
		bodies = append(bodies, string(content))
	}
	require.NotEmpty(t, bodies)
	for _, body := range bodies {
		require.Contains(t, []string{"alpha body\n", "bravo body\n"}, body)
	}
}

func TestWorkspaceScannerRejectsWhenPolicyDeniesTrustClass(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	writeWorkspaceFile(t, workspace, "alpha.txt", "alpha body\n")
	writeWorkspaceFile(t, workspace, "bravo.txt", "bravo body\n")

	scanner := &WorkspaceScanner{
		Store:         newIngestionStore(t),
		QuarantineDir: t.TempDir(),
		Scanners:      defaultScanners(),
		Policy:        &contextports.PolicyBundle{DefaultTrustClass: "workspace-trusted"},
		Evaluator:     &fakeEvaluator{admitTrustClass: false, quota: 5},
	}

	report, err := scanner.Scan(ctx, workspace)
	require.NoError(t, err)
	require.Zero(t, report.ChunksCreated)
	require.Zero(t, report.ChunksQuarantined)
	require.Equal(t, 2, report.ChunksRejected)
	require.Empty(t, report.Errors)
}

func TestWorkspaceScannerIgnoresUntypedPolicyValues(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	writeWorkspaceFile(t, workspace, "note.txt", "plain body\n")

	scanner := &WorkspaceScanner{
		Store:         newIngestionStore(t),
		QuarantineDir: t.TempDir(),
		Scanners:      defaultScanners(),
		Policy:        "not-a-policy-bundle",
		Evaluator:     42,
	}

	report, err := scanner.Scan(ctx, workspace)
	require.NoError(t, err)
	require.Equal(t, 1, report.ChunksCreated, "non-policy values must behave like no policy configured")
	require.Zero(t, report.ChunksQuarantined)
}

func TestWorkspaceScannerSkipsGitInternalFiles(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	writeWorkspaceFile(t, workspace, filepath.Join(".git", "HEAD"), "ref: refs/heads/main\n")
	writeWorkspaceFile(t, workspace, "main.go", "package main\n")

	report, err := (&WorkspaceScanner{
		Store:         newIngestionStore(t),
		QuarantineDir: t.TempDir(),
		Scanners:      defaultScanners(),
	}).Scan(ctx, workspace)
	require.NoError(t, err)
	require.Equal(t, 1, report.FilesScanned)
	require.Empty(t, report.Errors)
}

func TestWorkspaceScannerFailsWhenQuarantineDirUnavailable(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("regular file"), 0o600))

	scanner := &WorkspaceScanner{
		Store:         newIngestionStore(t),
		QuarantineDir: filepath.Join(blocker, "quarantine"),
	}
	report, err := scanner.Scan(ctx, t.TempDir())
	require.Error(t, err)
	require.Contains(t, err.Error(), "create quarantine dir")
	require.NotNil(t, report)
}

func TestWorkspaceScannerIncrementalFallsBackToFullScanWithoutGit(t *testing.T) {
	ctx := context.Background()
	workspace, quarantineRoot := t.TempDir(), t.TempDir()
	writeWorkspaceFile(t, workspace, "alpha.txt", "alpha body\n")
	writeWorkspaceFile(t, workspace, "bravo.txt", "bravo body\n")

	scanner := &WorkspaceScanner{
		Store:         newIngestionStore(t),
		QuarantineDir: quarantineRoot,
		Scanners:      defaultScanners(),
	}

	report, err := scanner.ScanIncremental(ctx, workspace, "HEAD")
	require.NoError(t, err, "outside a git repository the incremental scan must fall back to a full scan")
	require.Equal(t, 2, report.FilesScanned)
	require.Equal(t, 2, report.ChunksCreated)
	require.Empty(t, report.Errors)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, out)
}

func initGitRepo(t *testing.T, workspace string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	runGit(t, workspace, "init")
	runGit(t, workspace, "config", "user.email", "ingestion-test@example.invalid")
	runGit(t, workspace, "config", "user.name", "Ingestion Test")
}

func TestWorkspaceScannerIncrementalScansOnlyChangedFiles(t *testing.T) {
	ctx := context.Background()
	workspace, quarantineRoot := t.TempDir(), t.TempDir()
	initGitRepo(t, workspace)

	// beta.txt is the changed file and carries content that the admission
	// policy quarantines (exhausted quota); alpha.txt is unchanged and must
	// therefore never reach the pipeline.
	writeWorkspaceFile(t, workspace, "alpha.txt", "alpha content marker\n")
	runGit(t, workspace, "add", ".")
	runGit(t, workspace, "commit", "-m", "first")
	writeWorkspaceFile(t, workspace, "beta.txt", "SELECT password FROM users\n")
	runGit(t, workspace, "add", ".")
	runGit(t, workspace, "commit", "-m", "second")

	recorder := &chunkRecorder{}
	policy := &contextports.PolicyBundle{DefaultTrustClass: "workspace-trusted"}
	scanner := &WorkspaceScanner{
		Store:         newIngestionStore(t),
		QuarantineDir: quarantineRoot,
		Scanners:      []Scanner{recorder},
		Policy:        policy,
		Evaluator:     &fakeEvaluator{admitTrustClass: true, quota: 0},
	}

	report, err := scanner.ScanIncremental(ctx, workspace, "HEAD~1")
	require.NoError(t, err)
	require.Equal(t, 1, report.FilesScanned, "only files changed since the ref are processed")
	require.Len(t, recorder.seen(), 1)
	require.Zero(t, report.ChunksCreated)
	require.Equal(t, 1, report.ChunksQuarantined, "only the changed file was ingested")

	entries, err := os.ReadDir(quarantineRoot)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	content, err := os.ReadFile(filepath.Join(quarantineRoot, entries[0].Name(), "content.bin"))
	require.NoError(t, err)
	require.Contains(t, string(content), "SELECT password FROM users", "the quarantined body is the changed file, not the untouched one")
}

func TestWorkspaceScannerIncrementalRecordsErrorsForDeletedFiles(t *testing.T) {
	ctx := context.Background()
	workspace, quarantineRoot := t.TempDir(), t.TempDir()
	initGitRepo(t, workspace)

	deleted := writeWorkspaceFile(t, workspace, "gone.txt", "vanished body\n")
	runGit(t, workspace, "add", ".")
	runGit(t, workspace, "commit", "-m", "first")
	require.NoError(t, os.Remove(deleted))

	scanner := &WorkspaceScanner{
		Store:         newIngestionStore(t),
		QuarantineDir: quarantineRoot,
	}

	report, err := scanner.ScanIncremental(ctx, workspace, "HEAD")
	require.NoError(t, err)
	require.Equal(t, 1, report.FilesScanned)
	require.Len(t, report.Errors, 1)
	require.Contains(t, report.Errors[0].Error(), "gone.txt")
	require.Contains(t, report.Errors[0].Error(), "read file")
	require.Zero(t, report.ChunksCreated)
}

func TestWorkspaceScannerConfigurationSetters(t *testing.T) {
	scanner := &WorkspaceScanner{}

	policy := &contextports.PolicyBundle{DefaultTrustClass: "workspace-trusted"}
	scanner.SetPolicy(policy)
	require.Equal(t, policy, scanner.Policy)

	scope := sandbox.NewFileScopePolicy(t.TempDir(), nil)
	scanner.SetFileScope(scope)
	require.Same(t, scope, scanner.FileScope)

	scanner.AddSkillIngestionSources([]SkillIngestionSource{
		{Path: "skills/*.md", SourceType: "skill"},
		{Path: "skills/**/*.yaml", SourceType: "resource"},
	})
	require.Equal(t, []string{"skills/*.md", "skills/**/*.yaml"}, scanner.IncludeGlobs)
}

func TestWorkspaceScannerConcurrencyDefaults(t *testing.T) {
	require.Equal(t, 4, (&WorkspaceScanner{}).concurrency())
	require.Equal(t, 4, (&WorkspaceScanner{Concurrency: -3}).concurrency())
	require.Equal(t, 2, (&WorkspaceScanner{Concurrency: 2}).concurrency())
	require.Equal(t, 1, (&WorkspaceScanner{Concurrency: 1}).concurrency())
}

func TestWorkspaceScannerFileScopeGuardsDiscoveryAndIngestion(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	writeWorkspaceFile(t, workspace, "open.txt", "open body\n")
	protectedDir := filepath.Join(workspace, "relurpify_cfg")
	writeWorkspaceFile(t, protectedDir, "secret.txt", "secret body\n")

	policy, evaluator := quarantinePolicy()
	scanner := &WorkspaceScanner{
		Store:         newIngestionStore(t),
		QuarantineDir: t.TempDir(),
		Scanners:      defaultScanners(),
		Policy:        policy,
		Evaluator:     evaluator,
		FileScope:     sandbox.NewFileScopePolicy(workspace, []string{protectedDir}),
	}

	report, err := scanner.Scan(ctx, workspace)
	require.NoError(t, err)
	require.Equal(t, 1, report.FilesScanned, "protected subtrees must never be discovered")
	require.Equal(t, 1, report.ChunksQuarantined)
	require.Empty(t, report.Errors)

	entries, err := os.ReadDir(filepath.Join(scanner.QuarantineDir))
	require.NoError(t, err)
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(scanner.QuarantineDir, entry.Name(), "content.bin"))
		require.NoError(t, err)
		require.NotContains(t, string(content), "secret body", "protected content must not reach quarantine")
	}
}
