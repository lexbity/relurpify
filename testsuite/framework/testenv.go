package framework

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	regpkg "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/platform/fs"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// TestEnvironment provides a reusable test environment for framework integration tests.
// It owns all required framework state and ensures deterministic cleanup.
type TestEnvironment struct {
	// WorkspacePath is the temporary workspace directory for the test.
	WorkspacePath string

	// ManifestRoot is the root path for manifest resolution.
	ManifestRoot string

	// PermissionManager enforces runtime permissions.
	PermissionManager *authorization.PermissionManager

	// Registry manages tool capability registration and dispatch.
	Registry *regpkg.CapabilityRegistry

	// TelemetrySink captures telemetry events for assertion.
	TelemetrySink *recordingTelemetrySink

	// AuditSink captures audit records for assertion.
	AuditSink *recordingAuditSink

	// cleanup is the function that tears down the environment.
	cleanup func()

	// mu protects concurrent access to environment state.
	mu sync.RWMutex
}

// recordingTelemetrySink captures telemetry events in memory for test assertions.
type recordingTelemetrySink struct {
	events []telemetry.Event
	mu     sync.Mutex
}

// Emit records a telemetry event.
func (r *recordingTelemetrySink) Emit(event telemetry.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

// Events returns a copy of all captured events.
func (r *recordingTelemetrySink) Events() []telemetry.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	copies := make([]telemetry.Event, len(r.events))
	copy(copies, r.events)
	return copies
}

// Clear removes all captured events.
func (r *recordingTelemetrySink) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}

// recordingAuditSink captures audit records in memory for test assertions.
type recordingAuditSink struct {
	records []policy.AuditRecord
	mu      sync.Mutex
}

// Log records an audit entry.
func (r *recordingAuditSink) Log(ctx context.Context, record policy.AuditRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
	return nil
}

// Query filters audit records based on the supplied query.
func (r *recordingAuditSink) Query(ctx context.Context, filter policy.AuditQuery) ([]policy.AuditRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []policy.AuditRecord
	for _, record := range r.records {
		if filter.AgentID != "" && record.AgentID != filter.AgentID {
			continue
		}
		if filter.Type != "" && record.Type != filter.Type {
			continue
		}
		if filter.Action != "" && record.Action != filter.Action {
			continue
		}
		if !filter.TimeStart.IsZero() && record.Timestamp.Before(filter.TimeStart) {
			continue
		}
		if !filter.TimeEnd.IsZero() && record.Timestamp.After(filter.TimeEnd) {
			continue
		}
		if filter.Permission != "" && record.Permission != filter.Permission {
			continue
		}
		if filter.Result != "" && record.Result != filter.Result {
			continue
		}
		result = append(result, record)
	}
	return result, nil
}

// Records returns a copy of all captured audit records.
func (r *recordingAuditSink) Records() []policy.AuditRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	copies := make([]policy.AuditRecord, len(r.records))
	copy(copies, r.records)
	return copies
}

// Clear removes all captured records.
func (r *recordingAuditSink) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = nil
}

// NewTestEnvironment creates a new test environment with minimal but real framework state.
// The returned environment must be torn down by calling Cleanup().
func NewTestEnvironment(t *testing.T) *TestEnvironment {
	t.Helper()

	workspace, err := os.MkdirTemp("", "framework-suite-*")
	if err != nil {
		t.Fatalf("failed to create workspace directory: %v", err)
	}

	manifestRoot := filepath.Join(workspace, "manifests")
	if err := fs.MkdirAllSecure(manifestRoot); err != nil {
		_ = os.RemoveAll(workspace)
		t.Fatalf("failed to create manifest root: %v", err)
	}

	perms := policy.NewFileSystemPermissionSet(workspace, permissions.FileSystemRead, permissions.FileSystemWrite, permissions.FileSystemList)

	auditSink := &recordingAuditSink{}

	permManager, err := authorization.NewPermissionManager(workspace, perms, auditSink, nil)
	if err != nil {
		_ = os.RemoveAll(workspace)
		t.Fatalf("failed to create permission manager: %v", err)
	}

	registry := regpkg.NewRegistry()

	telemetrySink := &recordingTelemetrySink{}

	cleanup := func() {
		telemetrySink.Clear()
		auditSink.Clear()
		_ = os.RemoveAll(workspace)
	}

	env := &TestEnvironment{
		WorkspacePath:     workspace,
		ManifestRoot:      manifestRoot,
		PermissionManager: permManager,
		Registry:          registry,
		TelemetrySink:     telemetrySink,
		AuditSink:         auditSink,
		cleanup:           cleanup,
	}

	t.Cleanup(func() {
		env.Cleanup()
	})

	return env
}

// Cleanup tears down the test environment.
// This is called automatically by t.Cleanup, but can be called explicitly if needed.
func (e *TestEnvironment) Cleanup() {
	e.mu.Lock()

	cleanup := e.cleanup
	if cleanup == nil {
		e.mu.Unlock()
		return
	}
	e.cleanup = nil
	e.mu.Unlock()

	cleanup()
}
