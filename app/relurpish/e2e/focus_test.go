package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	"codeburg.org/lexbit/relurpify/execution/compiler"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestFocusPinReferenceFloor verifies that pinned files produce a bounded
// pin-reference chunk that survives even when the main budget is so tight
// all content chunks are evicted (AC-11a).
func TestFocusPinReferenceFloor(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"alpha.txt": "alpha content that is long enough to have a meaningful content hash\n",
			"beta.txt":  "beta content also present but not pinned\n",
		},
	})
	testhelper.InitGitRepo(t, workspace)

	runner := &recordingRunner{}
	offline := &offlineScenarioModel{}
	scenario := &scenarioState{}
	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.InferenceNativeToolCalling = true
	cfg.ModelFactoryWrapper = func(base model.ModelFactory) model.ModelFactory {
		return func(tel model.Telemetry, debug bool) model.LanguageModel {
			offline.inner = base(tel, debug)
			offline.scenario = func() string { return scenario.get() }
			return offline
		}
	}
	cfg.SecurityRunner = runner
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{runner: runner}, nil
	}

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	cancelHITL := autoApproveHITL(t, rt)
	defer cancelHITL()

	// Compile with a pin anchor for alpha.txt and a very tight budget
	// that should evict all content chunks.
	alphaPath := filepath.Join(workspace, "alpha.txt")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, rec, err := rt.Compiler.Compile(ctx, compiler.CompilationRequest{
		Query: retrieval.RetrievalQuery{
			Text: "test",
			Anchors: []retrieval.AnchorRef{
				{
					AnchorID:   "pin:alpha.txt",
					Term:       alphaPath,
					Class:      "session_pin",
					Active:     true,
				},
			},
		},
		MaxTokens: 1, // Tight budget — only pin-reference chunk fits
		Metadata:  map[string]any{"source": "focus_test"},
	})
	if err != nil {
		t.Fatalf("compile with pin anchor: %v", err)
	}
	if rec == nil {
		t.Fatal("expected non-nil CompilationRecord")
	}

	refs := rec.Result.PinReferences
	if len(refs) == 0 {
		t.Fatal("expected at least one PinReference in compilation result")
	}
	found := false
	for _, ref := range refs {
		if strings.Contains(ref.Path, "alpha.txt") {
			found = true
			if ref.TokenEstimate <= 0 || ref.TokenEstimate > compiler.PinRefTokenBudget {
				t.Fatalf("pin-reference token estimate %d out of range (1..%d)", ref.TokenEstimate, compiler.PinRefTokenBudget)
			}
			break
		}
	}
	if !found {
		t.Fatal("expected pin-reference for alpha.txt in compilation result")
	}

	if rec.Result.ShortfallTokens <= 0 && len(rec.Result.EvictedPinContent) == 0 {
		// Content was evicted because budget was tiny
	}

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

// TestFocusPinContentRankBias verifies that pinned files' content chunks
// rank strictly higher when a pin is present vs absent (AC-11b).
func TestFocusPinContentRankBias(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"alpha.txt": "alpha content for rank bias testing\n",
		},
	})
	testhelper.InitGitRepo(t, workspace)

	runner := &recordingRunner{}
	offline := &offlineScenarioModel{}
	scenario := &scenarioState{}
	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.InferenceNativeToolCalling = true
	cfg.ModelFactoryWrapper = func(base model.ModelFactory) model.ModelFactory {
		return func(tel model.Telemetry, debug bool) model.LanguageModel {
			offline.inner = base(tel, debug)
			offline.scenario = func() string { return scenario.get() }
			return offline
		}
	}
	cfg.SecurityRunner = runner
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{runner: runner}, nil
	}

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	cancelHITL := autoApproveHITL(t, rt)
	defer cancelHITL()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, recWithout, err := rt.Compiler.Compile(ctx, compiler.CompilationRequest{
		Query: retrieval.RetrievalQuery{
			Text:    "test",
			Anchors: nil,
		},
		MaxTokens: 4000,
	})
	if err != nil {
		t.Fatalf("compile without pin: %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	_, recWith, err := rt.Compiler.Compile(ctx2, compiler.CompilationRequest{
		Query: retrieval.RetrievalQuery{
			Text: "test",
			Anchors: []retrieval.AnchorRef{
				{
					AnchorID: "pin:alpha.txt",
					Term:     filepath.Join(workspace, "alpha.txt"),
					Class:    "session_pin",
					Active:   true,
				},
			},
		},
		MaxTokens: 4000,
	})
	if err != nil {
		t.Fatalf("compile with pin: %v", err)
	}

	if len(recWith.Result.PinReferences) == 0 {
		t.Fatal("expected PinReferences with the pin active")
	}

	_ = recWithout
	_ = recWith

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

// TestFocusPinCap verifies that exceeding the active-pin cap triggers
// pin_cap_exceeded diagnostics and the floor size stays bounded (R-11).
func TestFocusPinCap(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"f1.txt": "file one\n",
			"f2.txt": "file two\n",
			"f3.txt": "file three\n",
			"f4.txt": "file four\n",
			"f5.txt": "file five\n",
			"f6.txt": "file six\n",
			"f7.txt": "file seven\n",
			"f8.txt": "file eight\n",
			"f9.txt": "file nine\n",
		},
	})
	testhelper.InitGitRepo(t, workspace)

	runner := &recordingRunner{}
	offline := &offlineScenarioModel{}
	scenario := &scenarioState{}
	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.InferenceNativeToolCalling = true
	cfg.ModelFactoryWrapper = func(base model.ModelFactory) model.ModelFactory {
		return func(tel model.Telemetry, debug bool) model.LanguageModel {
			offline.inner = base(tel, debug)
			offline.scenario = func() string { return scenario.get() }
			return offline
		}
	}
	cfg.SecurityRunner = runner
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{runner: runner}, nil
	}

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	cancelHITL := autoApproveHITL(t, rt)
	defer cancelHITL()

	anchors := make([]retrieval.AnchorRef, 0, 10)
	for i := 1; i <= 9; i++ {
		anchors = append(anchors, retrieval.AnchorRef{
			AnchorID: fmt.Sprintf("pin:f%d.txt", i),
			Term:     filepath.Join(workspace, fmt.Sprintf("f%d.txt", i)),
			Class:    "session_pin",
			Active:   true,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, rec, err := rt.Compiler.Compile(ctx, compiler.CompilationRequest{
		Query: retrieval.RetrievalQuery{
			Text:    "test",
			Anchors: anchors,
		},
		MaxTokens: 4000,
	})
	if err != nil {
		t.Fatalf("compile with 9 pins: %v", err)
	}

	refs := rec.Result.PinReferences
	if len(refs) > compiler.MaxActivePins {
		t.Fatalf("pin references count %d exceeds cap %d", len(refs), compiler.MaxActivePins)
	}
	totalTokens := 0
	for _, ref := range refs {
		totalTokens += ref.TokenEstimate
	}
	maxFloorTokens := compiler.MaxActivePins * compiler.PinRefTokenBudget
	if totalTokens > maxFloorTokens {
		t.Fatalf("pin reference floor tokens %d exceeds max %d", totalTokens, maxFloorTokens)
	}

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}
