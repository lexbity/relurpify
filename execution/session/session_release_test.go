package session

import (
	"context"
	"testing"

	"go.uber.org/goleak"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// releaseRecordingManager records ReleaseSession calls.
type releaseRecordingManager struct {
	released []string
}

func (m *releaseRecordingManager) CheckFileAccess(context.Context, string, permissions.FileSystemAction, string) error {
	return nil
}

func (m *releaseRecordingManager) SetDecisionSink(telemetry.DecisionSink) {}

func (m *releaseRecordingManager) DefaultPolicy() string { return "ask" }

func (m *releaseRecordingManager) ReleaseSession(sessionID string) int {
	m.released = append(m.released, sessionID)
	return 0
}

// stopRecordingBroker records Stop calls.
type stopRecordingBroker struct{ stopped int }

func (b *stopRecordingBroker) Stop() { b.stopped++ }

// TestWorkspaceCloseReleasesSession: closing the workspace releases the
// registration's session-scoped grants and stops the HITL broker's sweeper —
// neither may outlive the session (FR-9).
func TestWorkspaceCloseReleasesSession(t *testing.T) {
	defer goleak.VerifyNone(t)
	perms := &releaseRecordingManager{}
	broker := &stopRecordingBroker{}
	w := &Workspace{
		Registration: &Registration{
			ID:          "agent-release-test",
			Permissions: perms,
			HITL:        broker,
		},
	}
	if err := w.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(perms.released) != 1 || perms.released[0] != "agent-release-test" {
		t.Fatalf("ReleaseSession calls = %v, want [agent-release-test]", perms.released)
	}
	if broker.stopped != 1 {
		t.Fatalf("broker Stop calls = %d, want 1", broker.stopped)
	}
}

// TestWorkspaceCloseWithoutSecurityLegs: a workspace without a registration
// closes cleanly (nil-safe wiring, no panics).
func TestWorkspaceCloseWithoutSecurityLegs(t *testing.T) {
	defer goleak.VerifyNone(t)
	w := &Workspace{}
	if err := w.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
