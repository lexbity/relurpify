package euclotest

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/euclo"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/gate"
	"github.com/lexcodex/relurpify/named/euclo/orchestrate"
	"github.com/lexcodex/relurpify/testutil/euclotestutil"
)

// --- From integration_test.go ---

func testEnv(t *testing.T) agentenv.AgentEnvironment {
	return testutil.Env(t)
}

func integrationAgent(t *testing.T) *euclo.Agent {
	t.Helper()
	return euclo.New(testutil.Env(t))
}

// --- From profile_controller_test.go ---

// stubProfileCapability is a configurable stub for profile controller tests.
type stubProfileCapability struct {
	id            string
	profiles      []string
	contract      euclotypes.ArtifactContract
	eligible      bool
	executeResult euclotypes.ExecutionResult
	executeCalled bool
	executeCount  int
}

func (s *stubProfileCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:   s.id,
		Name: s.id,
		Annotations: map[string]any{
			"supported_profiles": s.profiles,
		},
	}
}

func (s *stubProfileCapability) Contract() euclotypes.ArtifactContract {
	return s.contract
}

func (s *stubProfileCapability) Eligible(_ euclotypes.ArtifactState, _ euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	return euclotypes.EligibilityResult{Eligible: s.eligible, Reason: "stub"}
}

func (s *stubProfileCapability) Execute(_ context.Context, _ euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	s.executeCalled = true
	s.executeCount++
	return s.executeResult
}

func testEnvMinimal() agentenv.AgentEnvironment {
	return testutil.EnvMinimal()
}

func testEnvelope(state *core.Context) euclotypes.ExecutionEnvelope {
	if state == nil {
		state = core.NewContext()
	}
	return euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "pc-test-task",
			Instruction: "test instruction",
		},
		Mode:    euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"},
		State:   state,
	}
}

func testProfileController(caps *capabilities.EucloCapabilityRegistry) *orchestrate.ProfileController {
	return orchestrate.NewProfileController(
		orchestrate.AdaptCapabilityRegistry(caps),
		gate.DefaultPhaseGates(),
		testEnvMinimal(),
		euclotypes.DefaultExecutionProfileRegistry(),
		nil,
	)
}

// --- From coding_capability_registry_test.go ---

// stubCodingCapability is a test helper implementing EucloCodingCapability
// with configurable behavior.
type stubCodingCapability struct {
	id               string
	contract         euclotypes.ArtifactContract
	eligible         bool
	eligibleReason   string
	executeResult    euclotypes.ExecutionResult
	annotations      map[string]any
	executeCalled    bool
	eligibleArtState euclotypes.ArtifactState
}

func (s *stubCodingCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:          s.id,
		Name:        s.id,
		Annotations: s.annotations,
	}
}

func (s *stubCodingCapability) Contract() euclotypes.ArtifactContract {
	return s.contract
}

func (s *stubCodingCapability) Eligible(artifacts euclotypes.ArtifactState, _ euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	s.eligibleArtState = artifacts
	return euclotypes.EligibilityResult{Eligible: s.eligible, Reason: s.eligibleReason}
}

func (s *stubCodingCapability) Execute(_ context.Context, _ euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	s.executeCalled = true
	return s.executeResult
}

// --- From recovery_test.go ---

// capturingCapability wraps a stub and lets tests intercept Execute.
type capturingCapability struct {
	*stubProfileCapability
	onExecute func(context.Context, euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult
}

func (c *capturingCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	if c.onExecute != nil {
		return c.onExecute(ctx, env)
	}
	return c.stubProfileCapability.Execute(ctx, env)
}

// --- From artifacts_test.go ---

type workflowArtifactWriterStub struct {
	records []memory.WorkflowArtifactRecord
}

func (s *workflowArtifactWriterStub) UpsertWorkflowArtifact(_ context.Context, artifact memory.WorkflowArtifactRecord) error {
	s.records = append(s.records, artifact)
	return nil
}

type workflowArtifactReaderStub struct {
	records []memory.WorkflowArtifactRecord
}

func (s *workflowArtifactReaderStub) ListWorkflowArtifacts(_ context.Context, workflowID, runID string) ([]memory.WorkflowArtifactRecord, error) {
	out := make([]memory.WorkflowArtifactRecord, 0, len(s.records))
	for _, record := range s.records {
		if record.WorkflowID != workflowID {
			continue
		}
		if runID != "" && record.RunID != runID {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}
