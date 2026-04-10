package orchestrate

import (
	"context"
	"reflect"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

type phaseHandlerFunc func(context.Context, interaction.PhaseMachineContext) (interaction.PhaseOutcome, error)

func (f phaseHandlerFunc) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	return f(ctx, mc)
}

func TestRecoveryStackAndHelperMappings(t *testing.T) {
	stack := NewRecoveryStack()
	if !stack.CanAttempt() {
		t.Fatal("expected fresh recovery stack to allow attempts")
	}
	stack.Record(RecoveryAttempt{Level: RecoveryLevelCapability})
	stack.Record(RecoveryAttempt{Level: RecoveryLevelCapability})
	stack.Record(RecoveryAttempt{Level: RecoveryLevelCapability})
	if stack.CanAttempt() {
		t.Fatal("expected stack to be exhausted after max depth")
	}

	hint := euclotypes.RecoveryHint{
		Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "fallback-a",
		SuggestedParadigm:   "planning",
	}
	if got := fallbackCapabilitiesFromHint(hint); !reflect.DeepEqual(got, []string{"fallback-a"}) {
		t.Fatalf("unexpected fallback capabilities: %#v", got)
	}
	if got := fallbackCapabilitiesFromHint(euclotypes.RecoveryHint{SuggestedCapability: "a,b, c ; d"}); !reflect.DeepEqual(got, []string{"a,b, c ; d"}) {
		t.Fatalf("unexpected fallback capability suggestion handling: %#v", got)
	}
	if got := fallbackCapabilitiesFromHint(euclotypes.RecoveryHint{
		Context: map[string]any{"fallback_capabilities": []any{"x", "y", 3}},
	}); !reflect.DeepEqual(got, []string{"x", "y"}) {
		t.Fatalf("unexpected parsed fallback capabilities from context: %#v", got)
	}
	if got := firstFallbackCapability([]string{"x", "y"}); got != "x" {
		t.Fatalf("unexpected first fallback capability: %q", got)
	}
	if got := preferredFallbackProfile(euclotypes.RecoveryHint{Context: map[string]any{"preferred_profile": "cap"}}, []string{"one", "two"}); got != "cap" {
		t.Fatalf("unexpected preferred profile: %q", got)
	}
	if got := uniqueRecoveryStrings([]string{" a ", "a", "b", "", "b"}); !reflect.DeepEqual(got, []string{" a ", "a", "b"}) {
		t.Fatalf("unexpected unique recovery strings: %#v", got)
	}
}

func TestSetDefaultSnapshotFuncAndSnapshotFromEnv(t *testing.T) {
	original := defaultSnapshotFunc
	t.Cleanup(func() { defaultSnapshotFunc = original })

	defaultSnapshotFunc = func(reg interface{}) euclotypes.CapabilitySnapshot {
		if reg == nil {
			return euclotypes.CapabilitySnapshot{}
		}
		return euclotypes.CapabilitySnapshot{HasExecuteTools: true, ToolNames: []string{"demo"}}
	}

	env := euclotypes.ExecutionEnvelope{Registry: capability.NewRegistry()}
	got := snapshotFromEnv(env)
	if !got.HasExecuteTools || len(got.ToolNames) != 1 || got.ToolNames[0] != "demo" {
		t.Fatalf("unexpected snapshot from env: %#v", got)
	}

	SetDefaultSnapshotFunc(func(reg interface{}) euclotypes.CapabilitySnapshot {
		return euclotypes.CapabilitySnapshot{HasReadTools: true}
	})
	got = snapshotFromEnv(env)
	if !got.HasReadTools || got.HasExecuteTools {
		t.Fatalf("expected overridden snapshot func to apply, got %#v", got)
	}

	if got := snapshotFromEnv(euclotypes.ExecutionEnvelope{}); !reflect.DeepEqual(got, euclotypes.CapabilitySnapshot{}) {
		t.Fatalf("expected nil registry snapshot to be empty, got %#v", got)
	}
}

func TestAdaptCapabilityRegistry(t *testing.T) {
	if AdaptCapabilityRegistry(nil) != nil {
		t.Fatal("expected nil registry adapter for nil input")
	}

	capA := &stubCap{id: "cap-a", eligible: true, status: euclotypes.ExecutionStatusCompleted}
	reg := capabilities.NewEucloCapabilityRegistry()
	if err := reg.Register(capA); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	adapter := AdaptCapabilityRegistry(reg)
	if adapter == nil {
		t.Fatal("expected registry adapter")
	}
	if got := adapter.ForProfile("missing"); len(got) != 1 {
		t.Fatalf("expected adapter to surface registered capability for unknown profile, got %#v", got)
	}
	lookedUp, ok := adapter.Lookup("cap-a")
	if !ok || lookedUp == nil {
		t.Fatal("expected capability lookup through adapter to succeed")
	}
	if lookedUp.Descriptor().ID != "cap-a" {
		t.Fatalf("unexpected descriptor from adapter: %#v", lookedUp.Descriptor())
	}
}

func TestRecoveryControllerAttemptRecoveryBranches(t *testing.T) {
	t.Run("paradigm switch missing suggestion", func(t *testing.T) {
		rc := NewRecoveryController(nil, nil, nil, testutil.EnvMinimal())
		stack := NewRecoveryStack()
		failed := euclotypes.ExecutionResult{
			Status: euclotypes.ExecutionStatusFailed,
			Artifacts: []euclotypes.Artifact{{
				ProducerID: "cap-a",
				Kind:       euclotypes.ArtifactKindAnalyze,
			}},
		}
		got := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
			Strategy:          euclotypes.RecoveryStrategyParadigmSwitch,
			SuggestedParadigm: "",
		}, failed, testEnvelope(), stack)
		if got.Status != euclotypes.ExecutionStatusFailed {
			t.Fatalf("expected original failure, got %+v", got)
		}
		if len(stack.Attempts) != 1 || stack.Attempts[0].Success {
			t.Fatalf("expected one failed attempt, got %+v", stack.Attempts)
		}
	})

	t.Run("paradigm switch missing capability", func(t *testing.T) {
		rc := NewRecoveryController(recoveryStubRegistry{}, nil, nil, testutil.EnvMinimal())
		stack := NewRecoveryStack()
		failed := euclotypes.ExecutionResult{
			Status: euclotypes.ExecutionStatusFailed,
			Artifacts: []euclotypes.Artifact{{
				ProducerID: "cap-missing",
				Kind:       euclotypes.ArtifactKindAnalyze,
			}},
		}
		got := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
			Strategy:          euclotypes.RecoveryStrategyParadigmSwitch,
			SuggestedParadigm: "planning",
		}, failed, testEnvelope(), stack)
		if got.Status != euclotypes.ExecutionStatusFailed {
			t.Fatalf("expected original failure, got %+v", got)
		}
		if len(stack.Attempts) != 1 || stack.Attempts[0].Reason == "" {
			t.Fatalf("expected missing capability attempt to be recorded, got %+v", stack.Attempts)
		}
	})

	t.Run("paradigm switch success", func(t *testing.T) {
		capabilityRun := false
		capA := &stubCap{
			id:       "cap-a",
			eligible: true,
			status:   euclotypes.ExecutionStatusCompleted,
			executeFn: func(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
				capabilityRun = true
				if env.Task == nil || env.Task.Context["euclo.paradigm_override"] != "planning" {
					t.Fatalf("expected paradigm override in task context, got %#v", env.Task)
				}
				return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "recovered"}
			},
		}
		rc := NewRecoveryController(recoveryStubRegistry{byID: map[string]CapabilityI{"cap-a": capA}}, nil, nil, testutil.EnvMinimal())
		stack := NewRecoveryStack()
		failed := euclotypes.ExecutionResult{
			Status: euclotypes.ExecutionStatusFailed,
			Artifacts: []euclotypes.Artifact{{
				ProducerID: "cap-a",
				Kind:       euclotypes.ArtifactKindAnalyze,
			}},
			FailureInfo: &euclotypes.CapabilityFailure{ParadigmUsed: "react"},
		}
		env := testEnvelope()
		got := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
			Strategy:          euclotypes.RecoveryStrategyParadigmSwitch,
			SuggestedParadigm: "planning",
		}, failed, env, stack)
		if !capabilityRun {
			t.Fatal("expected capability to rerun under paradigm switch")
		}
		if got.Status != euclotypes.ExecutionStatusCompleted {
			t.Fatalf("expected successful recovery result, got %+v", got)
		}
		if len(stack.Attempts) != 1 || !stack.Attempts[0].Success {
			t.Fatalf("expected successful paradigm recovery attempt, got %+v", stack.Attempts)
		}
	})

	t.Run("nil stack", func(t *testing.T) {
		rc := NewRecoveryController(nil, nil, nil, testutil.EnvMinimal())
		failed := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
		got := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
			Strategy: euclotypes.RecoveryStrategyModeEscalation,
		}, failed, testEnvelope(), nil)
		if got.Status != euclotypes.ExecutionStatusFailed {
			t.Fatalf("expected original failure, got %+v", got)
		}
	})

	t.Run("exhausted stack", func(t *testing.T) {
		rc := NewRecoveryController(recoveryStubRegistry{}, nil, nil, testutil.EnvMinimal())
		stack := NewRecoveryStack()
		stack.MaxDepth = 1
		stack.Record(RecoveryAttempt{Level: RecoveryLevelCapability})
		failed := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
		got := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
			Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
			SuggestedCapability: "cap-x",
		}, failed, testEnvelope(), stack)
		if got.Status != euclotypes.ExecutionStatusFailed {
			t.Fatalf("expected original failure, got %+v", got)
		}
		if len(stack.Attempts) != 1 {
			t.Fatalf("expected exhausted stack to remain unchanged, got %+v", stack.Attempts)
		}
	})

	t.Run("capability fallback missing registry", func(t *testing.T) {
		rc := NewRecoveryController(nil, nil, nil, testutil.EnvMinimal())
		stack := NewRecoveryStack()
		failed := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
		got := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
			Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
			SuggestedCapability: "cap-x",
		}, failed, testEnvelope(), stack)
		if got.Status != euclotypes.ExecutionStatusFailed {
			t.Fatalf("expected original failure, got %+v", got)
		}
		if len(stack.Attempts) != 1 || stack.Attempts[0].Reason != "capability registry unavailable" {
			t.Fatalf("expected registry unavailable attempt, got %+v", stack.Attempts)
		}
	})

	t.Run("capability fallback no candidates", func(t *testing.T) {
		rc := NewRecoveryController(recoveryStubRegistry{}, nil, nil, testutil.EnvMinimal())
		stack := NewRecoveryStack()
		failed := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
		got := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
			Strategy: euclotypes.RecoveryStrategyCapabilityFallback,
		}, failed, testEnvelope(), stack)
		if got.Status != euclotypes.ExecutionStatusFailed {
			t.Fatalf("expected original failure, got %+v", got)
		}
		if len(stack.Attempts) != 1 || stack.Attempts[0].Reason != "no suggested capability" {
			t.Fatalf("expected no suggested capability attempt, got %+v", stack.Attempts)
		}
	})

	t.Run("capability fallback success", func(t *testing.T) {
		fallback := &stubCap{
			id:       "cap-fallback",
			eligible: true,
			status:   euclotypes.ExecutionStatusCompleted,
			summary:  "recovered",
		}
		rc := NewRecoveryController(recoveryStubRegistry{byID: map[string]CapabilityI{"cap-fallback": fallback}}, nil, nil, testutil.EnvMinimal())
		stack := NewRecoveryStack()
		failed := euclotypes.ExecutionResult{
			Status: euclotypes.ExecutionStatusFailed,
			Artifacts: []euclotypes.Artifact{{
				ProducerID: "cap-original",
				Kind:       euclotypes.ArtifactKindAnalyze,
			}},
		}
		got := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
			Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
			SuggestedCapability: "cap-fallback",
		}, failed, testEnvelope(), stack)
		if got.Status != euclotypes.ExecutionStatusCompleted {
			t.Fatalf("expected fallback success, got %+v", got)
		}
		if len(stack.Attempts) != 1 || !stack.Attempts[0].Success {
			t.Fatalf("expected successful capability fallback attempt, got %+v", stack.Attempts)
		}
	})

	t.Run("profile escalation missing registry", func(t *testing.T) {
		rc := NewRecoveryController(recoveryStubRegistry{}, nil, nil, testutil.EnvMinimal())
		stack := NewRecoveryStack()
		env := testEnvelope()
		failed := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
		got := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
			Strategy: euclotypes.RecoveryStrategyProfileEscalation,
		}, failed, env, stack)
		if got.Status != euclotypes.ExecutionStatusFailed {
			t.Fatalf("expected original failure, got %+v", got)
		}
		if len(stack.Attempts) != 1 || stack.Attempts[0].Reason != "profile registry unavailable" {
			t.Fatalf("expected profile registry unavailable attempt, got %+v", stack.Attempts)
		}
	})

	t.Run("profile escalation success", func(t *testing.T) {
		profiles := euclotypes.NewExecutionProfileRegistry()
		if err := profiles.Register(euclotypes.ExecutionProfileDescriptor{
			ProfileID:         "current",
			SupportedModes:    []string{"code"},
			FallbackProfiles:  []string{"fallback"},
			PhaseRoutes:       map[string]string{"analyze": "react"},
			RequiredArtifacts: []string{"euclo.intake"},
		}); err != nil {
			t.Fatalf("register current profile: %v", err)
		}
		if err := profiles.Register(euclotypes.ExecutionProfileDescriptor{
			ProfileID:         "fallback",
			SupportedModes:    []string{"code"},
			FallbackProfiles:  []string{},
			PhaseRoutes:       map[string]string{"analyze": "react"},
			RequiredArtifacts: []string{"euclo.intake"},
		}); err != nil {
			t.Fatalf("register fallback profile: %v", err)
		}
		fallbackCap := &stubCap{id: "cap-profile", eligible: true, status: euclotypes.ExecutionStatusCompleted, summary: "profile ok"}
		rc := NewRecoveryController(recoveryStubRegistry{byProfile: map[string][]CapabilityI{"fallback": {fallbackCap}}}, profiles, nil, testutil.EnvMinimal())
		stack := NewRecoveryStack()
		env := testEnvelope()
		env.Profile.ProfileID = "current"
		got := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
			Strategy: euclotypes.RecoveryStrategyProfileEscalation,
		}, euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}, env, stack)
		if got.Status != euclotypes.ExecutionStatusCompleted {
			t.Fatalf("expected profile escalation success, got %+v", got)
		}
		if len(stack.Attempts) != 1 || !stack.Attempts[0].Success {
			t.Fatalf("expected successful profile recovery attempt, got %+v", stack.Attempts)
		}
	})

	t.Run("mode escalation", func(t *testing.T) {
		rc := NewRecoveryController(nil, nil, nil, testutil.EnvMinimal())
		stack := NewRecoveryStack()
		failed := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed, Summary: "bad"}
		got := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
			Strategy: euclotypes.RecoveryStrategyModeEscalation,
		}, failed, testEnvelope(), stack)
		if got.Status != euclotypes.ExecutionStatusFailed {
			t.Fatalf("expected original failure, got %+v", got)
		}
		if got.RecoveryHint == nil || got.RecoveryHint.Context["requires_approval"] != true {
			t.Fatalf("expected mode escalation recovery hint, got %+v", got.RecoveryHint)
		}
		if len(stack.Attempts) != 1 || stack.Attempts[0].Level != RecoveryLevelMode {
			t.Fatalf("expected mode escalation attempt, got %+v", stack.Attempts)
		}
	})
}

func TestMaybeResumeInteractiveSessionAndUniqueStrings(t *testing.T) {
	resumeEmitter := interaction.NewTestFrameEmitter(interaction.ScriptedResponse{
		Kind:     string(interaction.FrameSessionResume),
		ActionID: "resume",
	})
	machine := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Emitter: resumeEmitter,
		Phases: []interaction.PhaseDefinition{
			{ID: "start", Handler: phaseHandlerFunc(func(context.Context, interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
				return interaction.PhaseOutcome{Advance: true}, nil
			})},
			{ID: "resume-point", Handler: phaseHandlerFunc(func(context.Context, interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
				return interaction.PhaseOutcome{Advance: true}, nil
			})},
		},
		Resolver: interaction.NewAgencyResolver(),
	})
	state := core.NewContext()
	state.Set("euclo.interaction_state", interaction.InteractionState{
		Mode:           "chat",
		CurrentPhase:   "resume-point",
		PhaseStates:    map[string]any{"resume.key": "value"},
		PhasesExecuted: []string{"start"},
		SkippedPhases:  []string{"skipped"},
		Selections:     map[string]string{"choice": "resume"},
	})

	if err := maybeResumeInteractiveSession(context.Background(), machine, state, "chat"); err != nil {
		t.Fatalf("maybeResumeInteractiveSession: %v", err)
	}
	if consumed, _ := state.Get("euclo.session_resume_consumed"); consumed != true {
		t.Fatalf("expected session resume to be consumed, got %#v", consumed)
	}
	if got := machine.CurrentPhase(); got != "resume-point" {
		t.Fatalf("expected resume jump to resume-point, got %q", got)
	}
	if got := machine.State()["resume.key"]; got != "value" {
		t.Fatalf("expected phase state restoration, got %#v", got)
	}

	if err := maybeResumeInteractiveSession(context.Background(), machine, state, "chat"); err != nil {
		t.Fatalf("second maybeResumeInteractiveSession: %v", err)
	}

	if got := uniqueStrings([]string{"alpha", "", "beta", "alpha", "beta"}); !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("unexpected unique strings: %#v", got)
	}
	if got := uniqueStrings(nil); got != nil {
		t.Fatalf("expected nil for empty uniqueStrings input, got %#v", got)
	}
}

func TestExecuteInteractiveAndTransitions(t *testing.T) {
	original := defaultSnapshotFunc
	t.Cleanup(func() { defaultSnapshotFunc = original })
	defaultSnapshotFunc = func(reg interface{}) euclotypes.CapabilitySnapshot {
		return euclotypes.CapabilitySnapshot{}
	}

	env := testEnvelope()
	env.State.Set("seed.key", "seed")
	env.State.Set("euclo.artifacts", []euclotypes.Artifact{{
		ID:         "seed-art",
		Kind:       euclotypes.ArtifactKindAnalyze,
		ProducerID: "seed-cap",
		Summary:    "seed",
	}})

	registry := interaction.NewModeMachineRegistry()
	registry.Register("code", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:     "code",
			Emitter:  emitter,
			Resolver: resolver,
			Phases: []interaction.PhaseDefinition{
				{
					ID: "inspect",
					Handler: phaseHandlerFunc(func(_ context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
						if !mc.Artifacts.Has(euclotypes.ArtifactKindAnalyze) {
							t.Fatal("expected seed artifact to be loaded into machine")
						}
						return interaction.PhaseOutcome{
							Advance: true,
							Artifacts: []euclotypes.Artifact{{
								ID:         "code-art",
								Kind:       euclotypes.ArtifactKindAnalyze,
								ProducerID: "code-cap",
								Summary:    "code",
							}},
							StateUpdates: map[string]any{"propose.items": []any{"one", "two"}},
						}, nil
					}),
				},
			},
		})
	})

	pc := NewProfileController(nil, nil, testutil.EnvMinimal(), nil, nil)
	result, detail, err := pc.ExecuteInteractive(context.Background(), registry, euclotypes.ModeResolution{ModeID: "code"}, env, &interaction.NoopEmitter{})
	if err != nil {
		t.Fatalf("ExecuteInteractive: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected interactive success, got %+v", result)
	}
	if detail == nil || len(detail.PhasesExecuted) != 1 || detail.PhasesExecuted[0] != "inspect" {
		t.Fatalf("unexpected interactive detail: %+v", detail)
	}
	if _, ok := env.State.Get("euclo.interaction_state"); !ok {
		t.Fatal("expected interaction state to be persisted")
	}
	if _, ok := env.State.Get("euclo.interaction_recording"); !ok {
		t.Fatal("expected interaction recording to be persisted")
	}
	if _, ok := env.State.Get("euclo.interaction_records"); !ok {
		t.Fatal("expected interaction records to be persisted")
	}
	if _, ok := env.State.Get("pipeline.plan"); !ok {
		t.Fatal("expected proposal items to be mirrored into pipeline.plan")
	}

	transitionEmitter := interaction.NewTestFrameEmitter(interaction.ScriptedResponse{
		Kind:     string(interaction.FrameTransition),
		ActionID: "accept",
	})
	transitionRegistry := interaction.NewModeMachineRegistry()
	transitionRegistry.Register("code", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:     "code",
			Emitter:  emitter,
			Resolver: resolver,
			Phases: []interaction.PhaseDefinition{
				{
					ID: "inspect",
					Handler: phaseHandlerFunc(func(_ context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
						return interaction.PhaseOutcome{
							Advance: true,
							Artifacts: []euclotypes.Artifact{{
								ID:         "code-art",
								Kind:       euclotypes.ArtifactKindAnalyze,
								ProducerID: "code-cap",
								Summary:    "code",
							}},
							Transition: "debug",
						}, nil
					}),
				},
			},
		})
	})
	transitionRegistry.Register("debug", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:     "debug",
			Emitter:  emitter,
			Resolver: resolver,
			Phases: []interaction.PhaseDefinition{
				{
					ID: "verify",
					Handler: phaseHandlerFunc(func(_ context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
						if !mc.Artifacts.Has(euclotypes.ArtifactKindAnalyze) {
							t.Fatal("expected carry-over artifact in transition target machine")
						}
						return interaction.PhaseOutcome{
							Advance: true,
							Artifacts: []euclotypes.Artifact{{
								ID:         "debug-art",
								Kind:       euclotypes.ArtifactKindPlan,
								ProducerID: "debug-cap",
								Summary:    "debug",
							}},
						}, nil
					}),
				},
			},
		})
	})

	env2 := testEnvelope()
	env2.State.Set("euclo.artifacts", []euclotypes.Artifact{{
		ID:         "seed-art",
		Kind:       euclotypes.ArtifactKindAnalyze,
		ProducerID: "seed-cap",
		Summary:    "seed",
	}})
	result, detail, err = pc.ExecuteInteractiveWithTransitions(context.Background(), transitionRegistry, euclotypes.ModeResolution{ModeID: "code"}, env2, transitionEmitter, 2)
	if err != nil {
		t.Fatalf("ExecuteInteractiveWithTransitions: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected transitioned interactive success, got %+v", result)
	}
	if detail == nil || detail.InteractionState.Mode != "debug" {
		t.Fatalf("expected final interaction state for debug mode, got %+v", detail)
	}
	if len(detail.PhasesExecuted) != 2 {
		t.Fatalf("expected both phase runs to be recorded, got %+v", detail.PhasesExecuted)
	}
	if got := uniqueStrings([]string{"inspect", "verify", "inspect", ""}); !reflect.DeepEqual(got, []string{"inspect", "verify"}) {
		t.Fatalf("unexpected deduped phases: %#v", got)
	}
}

func TestExecuteInteractive_WrapsNilEmitter(t *testing.T) {
	original := defaultSnapshotFunc
	t.Cleanup(func() { defaultSnapshotFunc = original })
	defaultSnapshotFunc = func(reg interface{}) euclotypes.CapabilitySnapshot {
		return euclotypes.CapabilitySnapshot{}
	}

	env := testEnvelope()
	registry := interaction.NewModeMachineRegistry()
	registry.Register("code", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:     "code",
			Emitter:  emitter,
			Resolver: resolver,
			Phases: []interaction.PhaseDefinition{
				{
					ID: "inspect",
					Handler: phaseHandlerFunc(func(_ context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
						return interaction.PhaseOutcome{Advance: true}, nil
					}),
				},
			},
		})
	})

	pc := NewProfileController(nil, nil, testutil.EnvMinimal(), nil, nil)
	result, detail, err := pc.ExecuteInteractive(context.Background(), registry, euclotypes.ModeResolution{ModeID: "code"}, env, nil)
	if err != nil {
		t.Fatalf("ExecuteInteractive with nil emitter: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if detail == nil || len(detail.PhasesExecuted) != 1 {
		t.Fatalf("unexpected detail from nil emitter path: %+v", detail)
	}
}

func TestExecuteInteractiveWithTransitions_MaxTransitionsExhaustion(t *testing.T) {
	original := defaultSnapshotFunc
	t.Cleanup(func() { defaultSnapshotFunc = original })
	defaultSnapshotFunc = func(reg interface{}) euclotypes.CapabilitySnapshot {
		return euclotypes.CapabilitySnapshot{}
	}

	transitionEmitter := interaction.NewTestFrameEmitter(interaction.ScriptedResponse{
		Kind:     string(interaction.FrameTransition),
		ActionID: "accept",
	})
	registry := interaction.NewModeMachineRegistry()
	registry.Register("code", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:     "code",
			Emitter:  emitter,
			Resolver: resolver,
			Phases: []interaction.PhaseDefinition{
				{
					ID: "inspect",
					Handler: phaseHandlerFunc(func(_ context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
						return interaction.PhaseOutcome{Advance: true, Transition: "debug"}, nil
					}),
				},
			},
		})
	})
	registry.Register("debug", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:     "debug",
			Emitter:  emitter,
			Resolver: resolver,
			Phases: []interaction.PhaseDefinition{
				{
					ID: "verify",
					Handler: phaseHandlerFunc(func(_ context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
						return interaction.PhaseOutcome{Advance: true, Transition: "code"}, nil
					}),
				},
			},
		})
	})

	pc := NewProfileController(nil, nil, testutil.EnvMinimal(), nil, nil)
	_, _, err := pc.ExecuteInteractiveWithTransitions(context.Background(), registry, euclotypes.ModeResolution{ModeID: "code"}, testEnvelope(), transitionEmitter, 1)
	if err == nil {
		t.Fatal("expected max transition exhaustion error")
	}
}

func TestRecoveryTraceAndFailureHelpers(t *testing.T) {
	stack := NewRecoveryStack()
	stack.Record(RecoveryAttempt{Level: RecoveryLevelMode, Strategy: euclotypes.RecoveryStrategyModeEscalation, From: "code", To: "debug", Reason: "escalate", Success: true})
	artifact := RecoveryTraceArtifact(stack, "producer-1")
	if artifact.Kind != euclotypes.ArtifactKindRecoveryTrace || artifact.ProducerID != "producer-1" {
		t.Fatalf("unexpected recovery trace artifact: %#v", artifact)
	}
	if payload, ok := artifact.Payload.(map[string]any); !ok || payload["exhausted"] != false || payload["max_depth"] != 3 {
		t.Fatalf("unexpected recovery trace payload: %#v", artifact.Payload)
	}
	if got := paradigmFromFailure(euclotypes.ExecutionResult{FailureInfo: &euclotypes.CapabilityFailure{ParadigmUsed: "react"}}); got != "react" {
		t.Fatalf("unexpected paradigm from failure: %q", got)
	}
	if got := producerIDFromFailure(euclotypes.ExecutionResult{Artifacts: []euclotypes.Artifact{{ProducerID: "cap-1"}}}); got != "cap-1" {
		t.Fatalf("unexpected producer from failure: %q", got)
	}
	if got := producerIDFromFailure(euclotypes.ExecutionResult{FailureInfo: &euclotypes.CapabilityFailure{Code: "cap-2"}}); got != "cap-2" {
		t.Fatalf("unexpected fallback producer id: %q", got)
	}
}

func TestControllerHelpersAndResultSynthesis(t *testing.T) {
	task := &core.Task{ID: "task-1", Instruction: "do work"}
	state := core.NewContext()
	artifacts := []euclotypes.Artifact{
		{ID: "a1", Kind: euclotypes.ArtifactKindAnalyze, Summary: "analyze", ProducerID: "cap-a", Payload: map[string]any{"x": 1}},
		{ID: "a2", Kind: euclotypes.ArtifactKindPlan, Summary: "plan", ProducerID: "cap-b", Payload: map[string]any{"y": 2}},
	}
	mergeCapabilityArtifactsToState(state, artifacts)
	if raw, ok := state.Get("euclo.artifacts"); !ok || len(raw.([]euclotypes.Artifact)) != 2 {
		t.Fatalf("expected artifacts merged into state, got %#v", raw)
	}
	if got := phaseExpectedArtifact("plan"); got != euclotypes.ArtifactKindPlan {
		t.Fatalf("unexpected phase artifact: %q", got)
	}
	if got := phaseExpectedArtifact("unknown"); got != "" {
		t.Fatalf("expected unknown phase to map to empty kind, got %q", got)
	}

	pcResult := &ProfileControllerResult{
		CapabilityIDs:  []string{"cap-a"},
		PhasesExecuted: []string{"plan"},
		PhaseRecords: []PhaseArtifactRecord{{
			Phase:             "plan",
			ArtifactsProduced: artifacts[:1],
			ArtifactsConsumed: []euclotypes.ArtifactKind{euclotypes.ArtifactKindIntake},
		}},
		EarlyStop:        true,
		EarlyStopPhase:   "verify",
		RecoveryAttempts: 2,
	}
	recordProfileControllerObservability(state, pcResult, euclotypes.ModeResolution{ModeID: "debug"}, euclotypes.ExecutionProfileSelection{ProfileID: "profile-a"})
	rawController, ok := state.Get("euclo.profile_controller")
	if !ok {
		t.Fatal("expected controller observability in state")
	}
	controllerMap, ok := rawController.(map[string]any)
	if !ok {
		t.Fatalf("unexpected controller observability shape: %#v", rawController)
	}
	if controllerMap["mode_id"] != "debug" || controllerMap["profile_id"] != "profile-a" {
		t.Fatalf("unexpected controller observability payload: %#v", controllerMap)
	}
	if phaseRecords, ok := controllerMap["phase_records"].([]map[string]any); !ok || len(phaseRecords) != 1 {
		t.Fatalf("unexpected controller phase records payload: %#v", controllerMap["phase_records"])
	}
	if rawPhaseRecords, ok := state.Get("euclo.profile_phase_records"); !ok {
		t.Fatal("expected phase records state in controller output")
	} else if got := rawPhaseRecords.([]map[string]any); len(got) != 1 || got[0]["phase"] != "plan" {
		t.Fatalf("unexpected profile phase records state: %#v", rawPhaseRecords)
	}

	records := buildProfileCapabilityPhaseRecords([]string{"plan", "verify"}, []euclotypes.ArtifactKind{euclotypes.ArtifactKindIntake}, artifacts)
	if len(records) != 2 || len(records[0].ArtifactsProduced) != 1 {
		t.Fatalf("unexpected phase records: %#v", records)
	}
	stateRecords := profilePhaseRecordsState(records)
	if len(stateRecords) != 2 {
		t.Fatalf("unexpected phase record state: %#v", stateRecords)
	}
	if got := artifactKindsFromState(euclotypes.NewArtifactState(artifacts)); !reflect.DeepEqual(got, []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze, euclotypes.ArtifactKindPlan}) {
		t.Fatalf("unexpected artifact kinds from state: %#v", got)
	}
	if got := artifactKindsFromArtifacts(artifacts); !reflect.DeepEqual(got, []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze, euclotypes.ArtifactKindPlan}) {
		t.Fatalf("unexpected artifact kinds from artifacts: %#v", got)
	}
	if got := artifactKindsToStrings([]euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze, ""}); !reflect.DeepEqual(got, []string{"euclo.analyze"}) {
		t.Fatalf("unexpected artifact kind strings: %#v", got)
	}
	if got := filterArtifactsByKind(artifacts, euclotypes.ArtifactKindPlan); len(got) != 1 || got[0].ID != "a2" {
		t.Fatalf("unexpected filtered artifacts: %#v", got)
	}
	if got := appendUniqueArtifactKinds([]euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan}, euclotypes.ArtifactKindPlan, euclotypes.ArtifactKindAnalyze); !reflect.DeepEqual(got, []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan, euclotypes.ArtifactKindAnalyze}) {
		t.Fatalf("unexpected appended unique artifact kinds: %#v", got)
	}

	if got := taskNodeID(task); got != "task-1" {
		t.Fatalf("unexpected task node id: %q", got)
	}
	if got := taskNodeID(nil); got != "euclo" {
		t.Fatalf("expected nil task node id to default to euclo, got %q", got)
	}

	if got := partialResult(task, pcResult); got == nil || got.NodeID != "task-1" || got.Data["status"] != "partial" {
		t.Fatalf("unexpected partial result: %#v", got)
	}
	failed := failedResult(task, euclotypes.ExecutionResult{Summary: "nope"}, pcResult)
	if failed == nil || failed.Success || failed.Data["status"] != "" {
		t.Fatalf("unexpected failed result: %#v", failed)
	}
	success := successResult(task, euclotypes.ExecutionResult{Summary: "ok"}, pcResult)
	if success == nil || !success.Success || success.Data["status"] != "" {
		t.Fatalf("unexpected success result: %#v", success)
	}
	completed := completedResult(task, pcResult)
	if completed == nil || !completed.Success || completed.Data["status"] != "completed" {
		t.Fatalf("unexpected completed result: %#v", completed)
	}
}

func TestSnapshotAndRecoveryHelpers(t *testing.T) {
	registry := capability.NewRegistry()
	snapshot := snapshotFromEnv(euclotypes.ExecutionEnvelope{Registry: registry})
	if snapshot.HasReadTools || snapshot.HasExecuteTools {
		t.Fatalf("expected empty registry snapshot, got %#v", snapshot)
	}

	phaseRoutes := map[string]string{"beta": "next", "alpha": "start"}
	if got := OrderedPhases(phaseRoutes, nil); !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("unexpected ordered phases: %#v", got)
	}

	if !shouldAttemptRecovery(euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusPartial, RecoveryHint: &euclotypes.RecoveryHint{Strategy: euclotypes.RecoveryStrategyModeEscalation}}) {
		t.Fatal("expected partial result with recovery hint to request recovery")
	}
	if shouldAttemptRecovery(euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, RecoveryHint: &euclotypes.RecoveryHint{Strategy: euclotypes.RecoveryStrategyModeEscalation}}) {
		t.Fatal("expected completed result not to request recovery")
	}
}

func TestResolveAndAdapterHelpers(t *testing.T) {
	capA := &stubCap{id: "cap-a", eligible: true, status: euclotypes.ExecutionStatusCompleted}
	capB := &stubCap{id: "cap-b", eligible: false, status: euclotypes.ExecutionStatusCompleted}
	pc := &ProfileController{
		Capabilities: stubRegistry{byProfile: map[string][]CapabilityI{"profile-a": {capA, capB}}},
	}
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindAnalyze}})
	snapshot := euclotypes.CapabilitySnapshot{}
	if got := pc.resolveProfileCapability("profile-a", state, snapshot); got == nil || got.Descriptor().ID != "cap-a" {
		t.Fatalf("unexpected profile capability resolution: %#v", got)
	}
	if got := pc.resolveCapabilityForPhase("analyze", "profile-a", state, snapshot); got == nil || got.Descriptor().ID != "cap-a" {
		t.Fatalf("unexpected phase capability resolution: %#v", got)
	}
	if got := pc.resolveFallbackCapability("analyze", "profile-a", "cap-a", state, snapshot); got != nil {
		t.Fatalf("expected fallback to skip excluded capability, got %#v", got)
	}

	reg := capabilities.NewEucloCapabilityRegistry()
	_ = reg.Register(capA)
	adapted := AdaptCapabilityRegistry(reg)
	if adapted == nil {
		t.Fatal("expected capability registry adapter")
	}
	if _, ok := adapted.Lookup("cap-a"); !ok {
		t.Fatal("expected adapter lookup to succeed")
	}
}
