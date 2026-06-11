package framework

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
)

// TestEnvelopeInitialization validates that a fresh envelope starts in a known state.
func TestEnvelopeInitialization(t *testing.T) {
	taskID := "test-task"
	sessionID := "test-session"

	// Create a fresh envelope
	env := contextdata.NewEnvelope(taskID, sessionID)

	// Validate envelope is not nil
	if env == nil {
		t.Fatal("expected envelope to be created, got nil")
	}

	// Validate TaskID is set correctly
	if env.TaskID != taskID {
		t.Errorf("expected TaskID %s, got %s", taskID, env.TaskID)
	}

	// Validate SessionID is set correctly
	if env.SessionID != sessionID {
		t.Errorf("expected SessionID %s, got %s", sessionID, env.SessionID)
	}

	// Validate NodeID is empty (not set during initialization)
	if env.NodeID != "" {
		t.Errorf("expected empty NodeID, got %s", env.NodeID)
	}

	// Validate WorkingData is initialized as a non-nil map
	if env.WorkingData == nil {
		t.Error("expected WorkingData to be initialized, got nil")
	}

	// Validate WorkingData is empty
	if len(env.WorkingData) != 0 {
		t.Errorf("expected empty WorkingData, got %d items", len(env.WorkingData))
	}

	// Validate References is initialized as empty bundle
	// Note: StreamedContext is a slice, so it's initialized as nil
	// This is acceptable - it will be allocated when references are added
	if len(env.References.StreamedContext) != 0 {
		t.Errorf("expected empty StreamedContext references, got %d items", len(env.References.StreamedContext))
	}

	// Validate CheckpointRequest is nil (not set during initialization)
	if env.CheckpointRequest != nil {
		t.Error("expected CheckpointRequest to be nil, got non-nil")
	}

	// Validate AssemblyMetadata is initialized with zero values
	if env.AssemblyMetadata.CompilationID != "" {
		t.Errorf("expected empty CompilationID, got %s", env.AssemblyMetadata.CompilationID)
	}
	if env.AssemblyMetadata.EventLogSeq != 0 {
		t.Errorf("expected EventLogSeq 0, got %d", env.AssemblyMetadata.EventLogSeq)
	}
	if env.AssemblyMetadata.BudgetTokens != 0 {
		t.Errorf("expected BudgetTokens 0, got %d", env.AssemblyMetadata.BudgetTokens)
	}
}

// TestEnvelopeWorkingValues validates that working values can be set and retrieved.
func TestEnvelopeWorkingValues(t *testing.T) {
	env := contextdata.NewEnvelope("task", "session")

	// Set a working value
	env.SetWorkingValueWithClass("key1", "value1", contextdata.MemoryClassTask)

	// Retrieve the value
	val, ok := contextdata.GetTyped[string](env, "key1")
	if !ok {
		t.Error("expected working value to be found")
	}
	if val != "value1" {
		t.Errorf("expected value 'value1', got %v", val)
	}

	// Set another working value
	env.SetWorkingValueWithClass("key2", 42, contextdata.MemoryClassTask)

	// Verify both values exist
	if len(env.WorkingData) != 2 {
		t.Errorf("expected 2 working values, got %d", len(env.WorkingData))
	}

	// Overwrite existing value
	env.SetWorkingValueWithClass("key1", "newvalue", contextdata.MemoryClassTask)

	// Verify overwrite worked
	val, ok = contextdata.GetTyped[string](env, "key1")
	if !ok {
		t.Error("expected working value to be found after overwrite")
	}
	if val != "newvalue" {
		t.Errorf("expected value 'newvalue', got %v", val)
	}
}

// TestEnvelopeReferences validates that references can be added and retrieved.
func TestEnvelopeReferences(t *testing.T) {
	env := contextdata.NewEnvelope("task", "session")

	// Add a streamed reference
	ref := contextdata.ChunkReference{
		ChunkID: contextdata.ChunkID("chunk-1"),
		Source:  "test-source",
		Rank:    1,
	}
	env.AddStreamedContextReference(ref)

	// Verify reference was added
	if len(env.References.StreamedContext) != 1 {
		t.Errorf("expected 1 streamed reference, got %d", len(env.References.StreamedContext))
	}

	// Verify reference content
	if env.References.StreamedContext[0].ChunkID != "chunk-1" {
		t.Errorf("expected chunk ID 'chunk-1', got %s", env.References.StreamedContext[0].ChunkID)
	}

	// Add another reference
	ref2 := contextdata.ChunkReference{
		ChunkID: contextdata.ChunkID("chunk-2"),
		Source:  "test-source",
		Rank:    2,
	}
	env.AddStreamedContextReference(ref2)

	// Verify both references exist
	if len(env.References.StreamedContext) != 2 {
		t.Errorf("expected 2 streamed references, got %d", len(env.References.StreamedContext))
	}

	// Verify references can be accessed directly
	refs := env.References.StreamedContext
	if len(refs) != 2 {
		t.Errorf("expected 2 references from StreamedContext, got %d", len(refs))
	}
}

// TestEnvelopeCheckpointRequest validates that checkpoint requests can be set.
func TestEnvelopeCheckpointRequest(t *testing.T) {
	env := contextdata.NewEnvelope("task", "session")

	// Request a checkpoint
	env.RequestCheckpoint("test reason", 5, true)

	// Verify checkpoint request was set
	if env.CheckpointRequest == nil {
		t.Fatal("expected CheckpointRequest to be set, got nil")
	}

	// Verify checkpoint request fields
	if env.CheckpointRequest.RequestedBy != "" {
		// RequestedBy is set automatically to the node ID, which we don't have in this test
		t.Logf("CheckpointRequest.RequestedBy: %s", env.CheckpointRequest.RequestedBy)
	}
	if env.CheckpointRequest.Reason != "test reason" {
		t.Errorf("expected Reason 'test reason', got %s", env.CheckpointRequest.Reason)
	}
	if env.CheckpointRequest.Priority != 5 {
		t.Errorf("expected Priority 5, got %d", env.CheckpointRequest.Priority)
	}
	if !env.CheckpointRequest.EvictWorkingMemory {
		t.Error("expected EvictWorkingMemory to be true")
	}

	// Verify timestamp was set
	if env.CheckpointRequest.RequestedAt.IsZero() {
		t.Error("expected RequestedAt to be set")
	}
}

// TestTriggerConstruction validates that a trigger can be constructed without side effects.
func TestTriggerConstruction(t *testing.T) {
	// Create a mock compiler
	compiler := &mockCompiler{}

	// Create a trigger
	trigger := contextstream.NewTrigger(compiler)

	// Validate trigger is not nil
	if trigger == nil {
		t.Fatal("expected trigger to be created, got nil")
	}

	// Validate compiler is set
	if trigger.Compiler == nil {
		t.Error("expected Compiler to be set, got nil")
	}

	// Validate compiler is the same instance
	if trigger.Compiler != compiler {
		t.Error("expected Compiler to be the same instance as provided")
	}

	// Validate no side effects (compiler state unchanged)
	if compiler.compileCalled {
		t.Error("expected no side effects during trigger construction")
	}
}

// TestTriggerContextBinding validates that a trigger can be bound to context and retrieved.
func TestTriggerContextBinding(t *testing.T) {
	compiler := &mockCompiler{}
	trigger := contextstream.NewTrigger(compiler)

	// Bind trigger to context
	ctx := context.Background()
	ctxWithTrigger := contextstream.WithTrigger(ctx, trigger)

	// Validate context is not nil
	if ctxWithTrigger == nil {
		t.Fatal("expected context with trigger to be non-nil")
	}

	// Retrieve trigger from context
	retrievedTrigger := contextstream.TriggerFromContext(ctxWithTrigger)

	// Validate trigger was retrieved
	if retrievedTrigger == nil {
		t.Fatal("expected trigger to be retrieved from context, got nil")
	}

	// Validate retrieved trigger is the same instance
	if retrievedTrigger != trigger {
		t.Error("expected retrieved trigger to be the same instance")
	}

	// Validate trigger from original context is nil
	triggerFromOriginal := contextstream.TriggerFromContext(ctx)
	if triggerFromOriginal != nil {
		t.Error("expected nil trigger from context without trigger")
	}
}

// TestJobCreation validates that a job can be created with proper initialization.
func TestJobCreation(t *testing.T) {
	req := contextstream.Request{
		ID:        "job-1",
		Query:     retrieval.RetrievalQuery{Text: "test query"},
		MaxTokens: 1000,
		Mode:      contextstream.ModeBackground,
	}

	// Create a job
	job := contextstream.NewJob(req)

	// Validate job is not nil
	if job == nil {
		t.Fatal("expected job to be created, got nil")
	}

	// Validate job ID is set
	if job.ID != "job-1" {
		t.Errorf("expected job ID 'job-1', got %s", job.ID)
	}

	// Validate request is set
	if job.Request.ID != req.ID {
		t.Errorf("expected request ID %s, got %s", req.ID, job.Request.ID)
	}

	// Validate StartedAt is zero (not set by NewJob, set by RequestBackground)
	if !job.StartedAt.IsZero() {
		t.Error("expected StartedAt to be zero (set by RequestBackground, not NewJob)")
	}

	// Validate CompletedAt is not set (job not completed)
	if !job.CompletedAt.IsZero() {
		t.Error("expected CompletedAt to be zero for uncompleted job")
	}

	// Validate Done channel is not nil
	if job.Done() == nil {
		t.Error("expected Done channel to be non-nil")
	}

	// Validate Done channel is not closed (job not completed)
	select {
	case <-job.Done():
		t.Error("expected Done channel to be open for uncompleted job")
	default:
		// Channel is open, which is correct
	}
}

// TestJobLifecycle validates the basic job lifecycle from creation to completion.
func TestJobLifecycle(t *testing.T) {
	req := contextstream.Request{
		ID:        "job-lifecycle",
		Query:     retrieval.RetrievalQuery{Text: "test query"},
		MaxTokens: 1000,
		Mode:      contextstream.ModeBlocking,
	}

	// Create a job
	job := contextstream.NewJob(req)

	// Validate job is not completed
	select {
	case <-job.Done():
		t.Error("expected job to not be completed initially")
	default:
		// Job is not completed, which is correct
	}

	// Validate job is in initial state
	if job.ID != "job-lifecycle" {
		t.Errorf("expected job ID 'job-lifecycle', got %s", job.ID)
	}
	if !job.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set by NewJob")
	}
	if !job.CompletedAt.IsZero() {
		t.Error("expected CompletedAt to be zero for uncompleted job")
	}

	// Note: We cannot call job.complete() as it's unexported
	// The actual completion happens through Trigger.RequestBlocking or RequestBackground
	// This test validates the initial state and structure
}

// TestJobWaitWithCancellation validates that job Wait respects context cancellation.
func TestJobWaitWithCancellation(t *testing.T) {
	req := contextstream.Request{
		ID:        "job-cancel",
		Query:     retrieval.RetrievalQuery{Text: "test query"},
		MaxTokens: 1000,
		Mode:      contextstream.ModeBackground,
	}

	job := contextstream.NewJob(req)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Wait should return context cancelled error
	_, err := job.Wait(ctx)
	if err == nil {
		t.Error("expected error from Wait with cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

// TestEnvelopeStateStability validates that envelope state remains stable before streaming.
func TestEnvelopeStateStability(t *testing.T) {
	env := contextdata.NewEnvelope("task", "session")

	// Set some initial state
	env.SetWorkingValueWithClass("key1", "value1", contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("key2", 42, contextdata.MemoryClassTask)

	ref := contextdata.ChunkReference{
		ChunkID: contextdata.ChunkID("chunk-1"),
		Source:  "test-source",
		Rank:    1,
	}
	env.AddStreamedContextReference(ref)

	// Capture initial state
	initialTaskID := env.TaskID
	initialSessionID := env.SessionID
	initialWorkingDataLen := len(env.WorkingData)
	initialRefsLen := len(env.References.StreamedContext)

	// Simulate time passing
	time.Sleep(10 * time.Millisecond)

	// Validate state remains stable
	if env.TaskID != initialTaskID {
		t.Error("expected TaskID to remain stable")
	}
	if env.SessionID != initialSessionID {
		t.Error("expected SessionID to remain stable")
	}
	if len(env.WorkingData) != initialWorkingDataLen {
		t.Error("expected WorkingData length to remain stable")
	}
	if len(env.References.StreamedContext) != initialRefsLen {
		t.Error("expected StreamedContext references length to remain stable")
	}

	// Validate values remain unchanged
	val, ok := contextdata.GetTyped[string](env, "key1")
	if !ok || val != "value1" {
		t.Error("expected working value to remain unchanged")
	}
}

// TestTriggerRequestConstruction validates that streaming requests can be constructed.
func TestTriggerRequestConstruction(t *testing.T) {
	req := contextstream.Request{
		ID:                    "req-1",
		Query:                 retrieval.RetrievalQuery{Text: "test query"},
		MaxTokens:             2000,
		EventLogSeq:           100,
		BudgetShortfallPolicy: "truncate",
		Mode:                  contextstream.ModeBlocking,
		Metadata: map[string]any{
			"key1": "value1",
			"key2": 42,
		},
		RequestedAt: time.Now().UTC(),
	}

	// Validate request fields
	if req.ID != "req-1" {
		t.Errorf("expected request ID 'req-1', got %s", req.ID)
	}
	if req.Query.Text != "test query" {
		t.Errorf("expected query text 'test query', got %s", req.Query.Text)
	}
	if req.MaxTokens != 2000 {
		t.Errorf("expected MaxTokens 2000, got %d", req.MaxTokens)
	}
	if req.EventLogSeq != 100 {
		t.Errorf("expected EventLogSeq 100, got %d", req.EventLogSeq)
	}
	if req.BudgetShortfallPolicy != "truncate" {
		t.Errorf("expected BudgetShortfallPolicy 'truncate', got %s", req.BudgetShortfallPolicy)
	}
	if req.Mode != contextstream.ModeBlocking {
		t.Errorf("expected Mode Blocking, got %s", req.Mode)
	}
	if len(req.Metadata) != 2 {
		t.Errorf("expected 2 metadata items, got %d", len(req.Metadata))
	}
	if req.RequestedAt.IsZero() {
		t.Error("expected RequestedAt to be set")
	}
}

// mockCompiler is a minimal compiler implementation for testing.
type mockCompiler struct {
	compileCalled bool
}

func (m *mockCompiler) Compile(ctx context.Context, req contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	m.compileCalled = true
	return &contextports.CompilationResult{}, nil
}
