package ingestion

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/fs"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	"codeburg.org/lexbit/relurpify/governance/identity"
)

var pipelineTestPrincipal = identity.SubjectRef{ID: "pipeline-tester"}

// fakeEvaluator is a local test double for the context-owned
// contextports.PolicyEvaluator interface; the production implementation lives
// in execution/context and is out of scope here.
type fakeEvaluator struct {
	admitTrustClass bool
	quota           int
}

func (f *fakeEvaluator) AdmitTrustClass(string) bool { return f.admitTrustClass }
func (f *fakeEvaluator) QuotaRemaining(string) int   { return f.quota }

// fixedSuspicionScanner always reports the same score, so admission outcomes
// can be driven deterministically without depending on chunk content.
type fixedSuspicionScanner struct {
	score float64
}

func (s *fixedSuspicionScanner) Name() string { return "fixed_suspicion" }

func (s *fixedSuspicionScanner) Scan(_ context.Context, _ TypedChunk) ScanResult {
	return ScanResult{SuspicionScore: s.score, Flags: []string{"fixed_suspicion"}}
}

func newIngestionStore(t *testing.T) *knowledge.ChunkStore {
	t.Helper()
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close(context.Background())) })
	return &knowledge.ChunkStore{Graph: engine}
}

func writeWorkspaceFile(t *testing.T, root, relPath, content string) string {
	t.Helper()
	path := filepath.Join(root, relPath)
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, fs.WriteFileSecure(path, []byte(content)))
	return path
}

func TestPipelineEmptyContentProducesZeroChunksWithoutPanicking(t *testing.T) {
	ctx := context.Background()
	store := newIngestionStore(t)

	for _, content := range []string{"", "\n", "\n\n\n"} {
		pipeline, err := AcquireFromUserInput(ctx, content, pipelineTestPrincipal, nil, nil, store)
		require.NoError(t, err)
		quarantineRoot := filepath.Join(t.TempDir(), "quarantine")
		pipeline.SetQuarantineDir(quarantineRoot)

		result, err := pipeline.Run(ctx)
		require.NoError(t, err, "content %q", content)
		require.Equal(t, DispositionCommit, result.Disposition, "content %q", content)
		require.Zero(t, result.ChunksCommitted, "content %q", content)
		require.Zero(t, result.ChunksQuarantined, "content %q", content)
		require.Zero(t, result.ChunksRejected, "content %q", content)
		require.Empty(t, result.ChunkIDs, "content %q", content)
		require.Zero(t, result.EventsEmitted, "content %q", content)
		require.Empty(t, result.QuarantinePath, "content %q", content)
	}
}

func TestAcquireFromUserInputCommitsChunk(t *testing.T) {
	ctx := context.Background()
	store := newIngestionStore(t)

	pipeline, err := AcquireFromUserInput(ctx, "first line\nsecond line\n", pipelineTestPrincipal, nil, nil, store)
	require.NoError(t, err)
	quarantineRoot := filepath.Join(t.TempDir(), "quarantine")
	pipeline.SetQuarantineDir(quarantineRoot)

	result, err := pipeline.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, DispositionCommit, result.Disposition)
	require.Equal(t, DispositionReason{Stage: "admission", Explanation: "no policy configured"}, result.Reason)
	require.Equal(t, 1, result.ChunksCommitted)
	require.Len(t, result.ChunkIDs, 1)
	require.Equal(t, 1, result.EventsEmitted)
	require.Zero(t, result.ChunksQuarantined)
	require.Zero(t, result.ChunksRejected)
	require.Empty(t, result.QuarantinePath)
	require.NoDirExists(t, quarantineRoot, "commit path must not create quarantine directories")

	chunk, ok, err := store.Load(result.ChunkIDs[0])
	require.NoError(t, err)
	require.True(t, ok, "committed chunk must be loadable from the store")
	require.Equal(t, "text", chunk.Body.Fields["content_type"])
	require.Equal(t, "user_input", string(chunk.AcquisitionMethod))
	require.Equal(t, pipelineTestPrincipal.ID, chunk.SourcePrincipal.ID)
	require.Equal(t, agentspec.TrustClassWorkspaceTrusted, chunk.TrustClass, "no policy means the safe default trust class")
	require.False(t, chunk.AcquiredAt.IsZero())
}

func TestAcquireFromFilePersistsOriginMetadata(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	path := writeWorkspaceFile(t, workspace, "src/notes.go", "package notes\n\nfunc Note() {}\n")
	store := newIngestionStore(t)

	pipeline, err := AcquireFromFile(ctx, path, pipelineTestPrincipal, nil, nil, store, nil)
	require.NoError(t, err)
	result, err := pipeline.Run(ctx)
	require.NoError(t, err)

	require.Equal(t, DispositionCommit, result.Disposition)
	require.Equal(t, 2, result.ChunksCommitted, "package clause chunk plus the Note function chunk")
	require.Len(t, result.ChunkIDs, 2)
	for _, id := range result.ChunkIDs {
		chunk, ok, err := store.Load(id)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "go", chunk.Body.Fields["content_type"])
		require.Equal(t, filepath.Clean(path), chunk.Body.Fields["file_path"])
		require.Equal(t, "file_read", string(chunk.AcquisitionMethod))
	}
	require.Greater(t, store.Revision(), uint64(0), "saves must bump the corpus revision")
}

func TestAcquireFromFileAllowsPathsWithinFileScope(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	path := writeWorkspaceFile(t, workspace, "allowed.txt", "inside the boundary\n")
	store := newIngestionStore(t)

	pipeline, err := AcquireFromFile(ctx, path, pipelineTestPrincipal, nil, nil, store,
		sandbox.NewFileScopePolicy(workspace, nil))
	require.NoError(t, err)

	result, err := pipeline.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, DispositionCommit, result.Disposition)
	require.Equal(t, 1, result.ChunksCommitted)
}

func TestAcquireFromFileFailsOnMissingFile(t *testing.T) {
	_, err := AcquireFromFile(context.Background(), filepath.Join(t.TempDir(), "missing.txt"),
		pipelineTestPrincipal, nil, nil, newIngestionStore(t), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read file")
}

func TestAcquireFromToolOutputUsesDetectedMIME(t *testing.T) {
	ctx := context.Background()
	store := newIngestionStore(t)

	pipeline, err := AcquireFromToolOutput(ctx, []byte("tool says hi\n"), pipelineTestPrincipal, nil, nil, store)
	require.NoError(t, err)

	result, err := pipeline.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, DispositionCommit, result.Disposition)
	require.Equal(t, 1, result.ChunksCommitted, "octet-stream hint falls back to the text parser producing one window")

	chunk, ok, err := store.Load(result.ChunkIDs[0])
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "text", chunk.Body.Fields["content_type"])
	require.Equal(t, "tool_output", string(chunk.AcquisitionMethod))
}

func TestAcquireFromCapabilityResultHonorsContentType(t *testing.T) {
	ctx := context.Background()
	store := newIngestionStore(t)
	source := []byte("package gen\n\nfunc One() {}\n\nfunc Two() {}\n")
	expected, err := (&GoParser{}).Parse(ctx, RawIngestion{Content: source})
	require.NoError(t, err)

	pipeline, err := AcquireFromCapabilityResult(ctx, source, "text/x-go", pipelineTestPrincipal, nil, nil, store)
	require.NoError(t, err)

	result, err := pipeline.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, DispositionCommit, result.Disposition)
	require.Equal(t, len(expected.ChunkBoundaries), result.ChunksCommitted)

	chunk, ok, err := store.Load(result.ChunkIDs[0])
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "go", chunk.Body.Fields["content_type"])
	require.Equal(t, "capability_result", string(chunk.AcquisitionMethod))
}

func TestPipelineQuarantinesWhenQuotaExhausted(t *testing.T) {
	ctx := context.Background()
	store := newIngestionStore(t)
	policy := &contextports.PolicyBundle{DefaultTrustClass: "workspace-trusted"}
	quarantineRoot := t.TempDir()

	pipeline, err := AcquireFromUserInput(ctx, "hello\nworld\n", pipelineTestPrincipal, policy,
		&fakeEvaluator{admitTrustClass: true, quota: 0}, store)
	require.NoError(t, err)
	pipeline.SetQuarantineDir(quarantineRoot)

	result, err := pipeline.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, DispositionQuarantine, result.Disposition)
	require.Equal(t, DispositionReason{Stage: "admission", Explanation: "quota exceeded"}, result.Reason)
	require.Equal(t, 1, result.ChunksQuarantined)
	require.Zero(t, result.ChunksCommitted)
	require.Empty(t, result.ChunkIDs)
	require.Equal(t, 1, result.EventsEmitted)
	require.DirExists(t, result.QuarantinePath)
	require.Equal(t, quarantineRoot, filepath.Dir(result.QuarantinePath), "quarantine entry lives under the configured root")

	content, err := os.ReadFile(filepath.Join(result.QuarantinePath, "content.bin"))
	require.NoError(t, err)
	require.Equal(t, "hello\nworld\n", string(content))

	reason, err := os.ReadFile(filepath.Join(result.QuarantinePath, "reason.txt"))
	require.NoError(t, err)
	require.Contains(t, string(reason), "Content type: text")
	require.Contains(t, string(reason), "Chunks: 1")
}

func TestPipelineQuarantinesHighSuspicionContent(t *testing.T) {
	ctx := context.Background()
	store := newIngestionStore(t)
	policy := &contextports.PolicyBundle{DefaultTrustClass: "workspace-trusted"}
	quarantineRoot := t.TempDir()

	pipeline, err := AcquireFromUserInput(ctx, "hello\nworld\n", pipelineTestPrincipal, policy,
		&fakeEvaluator{admitTrustClass: true, quota: 7}, store)
	require.NoError(t, err)
	pipeline.SetQuarantineDir(quarantineRoot)
	pipeline.SetScanners([]Scanner{&fixedSuspicionScanner{score: 0.8}})

	result, err := pipeline.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, DispositionQuarantine, result.Disposition)
	require.Equal(t, "admission", result.Reason.Stage)
	require.Equal(t, "high suspicion score: 0.80", result.Reason.Explanation)
	require.Equal(t, 1, result.ChunksQuarantined)
	require.Zero(t, result.ChunksCommitted)
	require.DirExists(t, result.QuarantinePath)
}

func TestPipelineSuspicionAtOrBelowThresholdCommits(t *testing.T) {
	ctx := context.Background()
	store := newIngestionStore(t)
	policy := &contextports.PolicyBundle{DefaultTrustClass: "workspace-trusted"}

	pipeline, err := AcquireFromUserInput(ctx, "hello\nworld\n", pipelineTestPrincipal, policy,
		&fakeEvaluator{admitTrustClass: true, quota: 7}, store)
	require.NoError(t, err)
	pipeline.SetQuarantineDir(filepath.Join(t.TempDir(), "quarantine"))
	pipeline.SetScanners([]Scanner{&fixedSuspicionScanner{score: 0.7}})

	result, err := pipeline.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, DispositionCommit, result.Disposition, "0.70 does not exceed the 0.7 quarantine threshold")
	require.Equal(t, "passed all checks", result.Reason.Explanation)
	require.Equal(t, 1, result.ChunksCommitted)
}

func TestPipelineRejectsWhenTrustClassDenied(t *testing.T) {
	ctx := context.Background()
	store := newIngestionStore(t)
	policy := &contextports.PolicyBundle{DefaultTrustClass: "workspace-trusted"}

	pipeline, err := AcquireFromUserInput(ctx, "hello\nworld\n", pipelineTestPrincipal, policy,
		&fakeEvaluator{admitTrustClass: false, quota: 5}, store)
	require.NoError(t, err)
	quarantineRoot := filepath.Join(t.TempDir(), "quarantine")
	pipeline.SetQuarantineDir(quarantineRoot)

	result, err := pipeline.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, DispositionReject, result.Disposition)
	require.Equal(t, DispositionReason{Stage: "admission", Explanation: "trust class rejected"}, result.Reason)
	require.Equal(t, 1, result.ChunksRejected)
	require.Zero(t, result.ChunksCommitted)
	require.Zero(t, result.ChunksQuarantined)
	require.Empty(t, result.QuarantinePath)
	require.NoDirExists(t, quarantineRoot)
}

func TestPipelineCommitStoresNormalizedTrustClass(t *testing.T) {
	ctx := context.Background()
	store := newIngestionStore(t)
	policy := &contextports.PolicyBundle{DefaultTrustClass: "WORKSPACE_TRUSTED"}

	pipeline, err := AcquireFromUserInput(ctx, "body text\n", pipelineTestPrincipal, policy,
		&fakeEvaluator{admitTrustClass: true, quota: 5}, store)
	require.NoError(t, err)

	result, err := pipeline.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, DispositionCommit, result.Disposition)
	require.Equal(t, DispositionReason{Stage: "admission", Explanation: "passed all checks"}, result.Reason)

	chunk, ok, err := store.Load(result.ChunkIDs[0])
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, agentspec.TrustClassWorkspaceTrusted, chunk.TrustClass, "policy trust class must be normalized before persisting")
}

func TestPipelineMultipleWindowsCommitDistinctChunks(t *testing.T) {
	ctx := context.Background()
	store := newIngestionStore(t)
	content := strings.Repeat("row of text\n", 120) // 121 lines -> 3 default windows

	pipeline, err := AcquireFromUserInput(ctx, content, pipelineTestPrincipal, nil, nil, store)
	require.NoError(t, err)

	result, err := pipeline.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, result.ChunksCommitted)
	require.Len(t, result.ChunkIDs, 3)
	requireUnique := make(map[knowledge.ChunkID]struct{}, len(result.ChunkIDs))
	for _, id := range result.ChunkIDs {
		requireUnique[id] = struct{}{}
		_, ok, err := store.Load(id)
		require.NoError(t, err)
		require.True(t, ok)
	}
	require.Len(t, requireUnique, 3, "distinct windows must produce distinct chunk ids")
	require.Equal(t, 3, result.EventsEmitted)
}

func TestPipelineCommitFailsWithoutStore(t *testing.T) {
	ctx := context.Background()
	pipeline, err := AcquireFromUserInput(ctx, "body text\n", pipelineTestPrincipal, nil, nil, nil)
	require.NoError(t, err)

	result, err := pipeline.Run(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "save chunk")
	require.ErrorIs(t, result.Error, err)
	require.Equal(t, DispositionCommit, result.Disposition, "disposition was decided before the commit failure")
}

func TestPipelineQuarantineWriteFailureSurfacesError(t *testing.T) {
	ctx := context.Background()
	store := newIngestionStore(t)
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	require.NoError(t, fs.WriteFileSecure(blocker, []byte("a regular file blocks mkdir below it")))
	policy := &contextports.PolicyBundle{DefaultTrustClass: "workspace-trusted"}

	pipeline, err := AcquireFromUserInput(ctx, "hello\nworld\n", pipelineTestPrincipal, policy,
		&fakeEvaluator{admitTrustClass: true, quota: 0}, store)
	require.NoError(t, err)
	pipeline.SetQuarantineDir(filepath.Join(blocker, "quarantine"))

	result, err := pipeline.Run(ctx)
	require.Error(t, err)
	require.Error(t, result.Error, "result must carry the stage 6 error")
	require.Contains(t, err.Error(), "create quarantine dir")
	require.Equal(t, DispositionQuarantine, result.Disposition)
}

func TestDefaultTrustClassNormalization(t *testing.T) {
	require.Equal(t, agentspec.TrustClassWorkspaceTrusted, defaultTrustClass(nil))
	require.Equal(t, agentspec.TrustClassWorkspaceTrusted, defaultTrustClass(&contextports.PolicyBundle{}))
	require.Equal(t, agentspec.TrustClassWorkspaceTrusted,
		defaultTrustClass(&contextports.PolicyBundle{DefaultTrustClass: "WORKSPACE_TRUSTED"}))
	require.Equal(t, agentspec.TrustClass("remote-declared"),
		defaultTrustClass(&contextports.PolicyBundle{DefaultTrustClass: "Remote_Declared"}))
}

func TestDetectMIMETypeClassification(t *testing.T) {
	cases := []struct {
		path    string
		content []byte
		want    string
	}{
		{"main.go", nil, "text/x-go"},
		{"script.py", nil, "text/x-python"},
		{"lib.rs", nil, "text/x-rust"},
		{"app.js", nil, "text/x-javascript"},
		{"app.ts", nil, "text/x-javascript"},
		{"comp.tsx", nil, "text/x-javascript"},
		{"README.MD", nil, "text/markdown"},
		{"notes.txt", nil, "text/plain"},
		{"data.json", nil, "application/json"},
		{"conf.yaml", nil, "application/yaml"},
		{"conf.yml", nil, "application/yaml"},
		{"archive.zip", nil, "application/octet-stream"},
		{"", []byte("plain"), "application/octet-stream"},
		{"", []byte{0x7f, 'E'}, "application/octet-stream"},
		{"", []byte{0x7f, 'E', 'L'}, "application/octet-stream"},
		{"", []byte{0x7f, 'x'}, "application/octet-stream"},
		{"", []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}, "application/x-elf"},
		{"", []byte{0x89, 'P', 'N', 'G', 0x0d}, "image/png"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, detectMIMEType(tc.path, tc.content), "path %q content %v", tc.path, tc.content)
	}
}

func TestIsBinaryContentClassification(t *testing.T) {
	cases := []struct {
		name    string
		content []byte
		want    bool
	}{
		{"plain text", []byte("hello world\nsecond line\n"), false},
		{"empty", nil, false},
		{"null byte", []byte("a\x00b"), true},
		{"mostly control bytes", append(bytes.Repeat([]byte{0x01}, 5), []byte("abcde")...), true},
		{"sparse control bytes", append(bytes.Repeat([]byte{0x02}, 1), []byte(strings.Repeat("a", 19))...), false},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, isBinaryContent(tc.content), tc.name)
	}
}
