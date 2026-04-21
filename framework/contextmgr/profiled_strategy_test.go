package contextmgr

import (
	"reflect"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

func TestProfiledStrategyMatchesPresetConcreteStrategies(t *testing.T) {
	taskWithFiles := &core.Task{
		Instruction: "Refactor pkg/foo.go and pkg/bar.go",
	}
	taskNoFiles := &core.Task{
		Instruction: "Search the codebase for authentication issues",
	}
	budget := core.NewContextBudget(12000)

	items := []core.ContextItem{
		&testContextItem{id: "low", tokens: 10, relevance: 0.1, age: 5 * time.Hour, itemType: core.ContextTypeObservation},
		&testContextItem{id: "mid", tokens: 10, relevance: 0.5, age: time.Hour, itemType: core.ContextTypeObservation},
		&testContextItem{id: "high", tokens: 10, relevance: 0.9, age: 10 * time.Minute, itemType: core.ContextTypeObservation},
	}

	t.Run("aggressive", func(t *testing.T) {
		expected := NewAggressiveStrategy()
		strategy := NewStrategyFromProfile(AggressiveProfile)

		gotReq, err := strategy.SelectContext(taskWithFiles, budget)
		if err != nil {
			t.Fatalf("SelectContext: %v", err)
		}
		wantReq, err := expected.SelectContext(taskWithFiles, budget)
		if err != nil {
			t.Fatalf("expected SelectContext: %v", err)
		}
		if !reflect.DeepEqual(gotReq, wantReq) {
			t.Fatalf("unexpected aggressive request:\n got: %#v\nwant: %#v", gotReq, wantReq)
		}
		shared := core.NewSharedContext(core.NewContext(), budget, nil)
		for i := 0; i < 12; i++ {
			shared.AddInteraction("assistant", "history", nil)
		}
		if strategy.ShouldCompress(shared) != expected.ShouldCompress(shared) {
			t.Fatal("unexpected aggressive compress result")
		}
		for _, relevance := range []float64{0.95, 0.91, 0.71, 0.51, 0.49} {
			if got, want := strategy.DetermineDetailLevel("file.go", relevance), expected.DetermineDetailLevel("file.go", relevance); got != want {
				t.Fatalf("unexpected aggressive detail level for %v: got %v want %v", relevance, got, want)
			}
		}
		if got, want := strategy.ShouldExpandContext(shared, &core.Result{Success: false, Data: map[string]any{"error_type": "insufficient_context"}}), expected.ShouldExpandContext(shared, &core.Result{Success: false, Data: map[string]any{"error_type": "insufficient_context"}}); got != want {
			t.Fatal("unexpected aggressive expand result")
		}
		if !reflect.DeepEqual(strategy.PrioritizeContext(items), expected.PrioritizeContext(items)) {
			t.Fatal("unexpected aggressive prioritization")
		}
	})

	t.Run("conservative-files", func(t *testing.T) {
		expected := NewConservativeStrategy()
		strategy := NewStrategyFromProfile(ConservativeProfile)

		gotReq, err := strategy.SelectContext(taskWithFiles, budget)
		if err != nil {
			t.Fatalf("SelectContext: %v", err)
		}
		wantReq := &ContextRequest{
			Files: []FileRequest{
				{Path: "pkg/foo.go", DetailLevel: DetailDetailed, Priority: 0, Pinned: true},
				{Path: "pkg/bar.go", DetailLevel: DetailDetailed, Priority: 0, Pinned: true},
			},
			ASTQueries: []ASTQuery{
				{Type: ASTQueryListSymbols, Filter: ASTFilter{ExportedOnly: false}},
				{Type: ASTQueryGetDependencies, Symbol: "pkg/foo.go"},
				{Type: ASTQueryGetDependencies, Symbol: "pkg/bar.go"},
			},
			SearchQueries: []SearchQuery{
				{Text: taskWithFiles.Instruction, Mode: SearchHybrid, MaxResults: 20},
			},
			MemoryQueries: []MemoryQuery{
				{Scope: memory.MemoryScopeProject, Query: taskWithFiles.Instruction, MaxResults: 10},
			},
			MaxTokens: budget.AvailableForContext * 3 / 4,
		}
		if !reflect.DeepEqual(gotReq, wantReq) {
			t.Fatalf("unexpected conservative request with files:\n got: %#v\nwant: %#v", gotReq, wantReq)
		}
		shared := core.NewSharedContext(core.NewContext(), budget, nil)
		for i := 0; i < 12; i++ {
			shared.AddInteraction("assistant", "history", nil)
		}
		if strategy.ShouldCompress(shared) != expected.ShouldCompress(shared) {
			t.Fatal("unexpected conservative compress result")
		}
		for _, relevance := range []float64{0.9, 0.81, 0.61, 0.51, 0.49} {
			if got, want := strategy.DetermineDetailLevel("file.go", relevance), expected.DetermineDetailLevel("file.go", relevance); got != want {
				t.Fatalf("unexpected conservative detail level for %v: got %v want %v", relevance, got, want)
			}
		}
		if got, want := strategy.ShouldExpandContext(shared, &core.Result{Success: true, Data: map[string]any{"tool_used": "search"}}), expected.ShouldExpandContext(shared, &core.Result{Success: true, Data: map[string]any{"tool_used": "search"}}); got != want {
			t.Fatal("unexpected conservative expand result")
		}
		if !reflect.DeepEqual(strategy.PrioritizeContext(items), expected.PrioritizeContext(items)) {
			t.Fatal("unexpected conservative prioritization")
		}
	})

	t.Run("conservative-no-files", func(t *testing.T) {
		expected := NewConservativeStrategy()
		gotReq, err := NewStrategyFromProfile(ConservativeProfile).SelectContext(taskNoFiles, budget)
		if err != nil {
			t.Fatalf("SelectContext: %v", err)
		}
		wantReq, err := expected.SelectContext(taskNoFiles, budget)
		if err != nil {
			t.Fatalf("expected SelectContext: %v", err)
		}
		if !reflect.DeepEqual(gotReq, wantReq) {
			t.Fatalf("unexpected conservative request without files:\n got: %#v\nwant: %#v", gotReq, wantReq)
		}
	})

	t.Run("balanced", func(t *testing.T) {
		strategy := NewStrategyFromProfile(BalancedProfile)
		gotReq, err := strategy.SelectContext(taskWithFiles, budget)
		if err != nil {
			t.Fatalf("SelectContext: %v", err)
		}
		wantReq := &ContextRequest{
			Files: []FileRequest{
				{Path: "pkg/foo.go", DetailLevel: DetailConcise, Priority: 0, Pinned: true},
				{Path: "pkg/bar.go", DetailLevel: DetailConcise, Priority: 0, Pinned: true},
			},
			ASTQueries: []ASTQuery{
				{Type: ASTQueryListSymbols, Filter: ASTFilter{ExportedOnly: true}},
			},
			SearchQueries: []SearchQuery{
				{Text: ExtractKeywords(taskWithFiles.Instruction), Mode: SearchHybrid, MaxResults: 10},
			},
			MaxTokens: budget.AvailableForContext / 2,
		}
		if !reflect.DeepEqual(gotReq, wantReq) {
			t.Fatalf("unexpected balanced request:\n got: %#v\nwant: %#v", gotReq, wantReq)
		}
		shared := core.NewSharedContext(core.NewContext(), budget, nil)
		for i := 0; i < 12; i++ {
			shared.AddInteraction("assistant", "history", nil)
		}
		if strategy.ShouldCompress(shared) != (len(shared.History()) > 10) {
			t.Fatal("unexpected balanced compress result")
		}
		for _, relevance := range []float64{0.95, 0.86, 0.61, 0.59} {
			want := DetailConcise
			switch {
			case relevance >= 0.85:
				want = DetailFull
			case relevance >= 0.6:
				want = DetailDetailed
			}
			if got := strategy.DetermineDetailLevel("file.go", relevance); got != want {
				t.Fatalf("unexpected balanced detail level for %v: got %v want %v", relevance, got, want)
			}
		}
		if got := strategy.ShouldExpandContext(shared, &core.Result{Success: true, Data: map[string]any{"llm_output": "we need more information"}}); !got {
			t.Fatal("unexpected balanced expand result")
		}
		wantItems := []core.ContextItem{
			&testContextItem{id: "high", tokens: 10, relevance: 0.9, age: 10 * time.Minute, itemType: core.ContextTypeObservation},
			&testContextItem{id: "mid", tokens: 10, relevance: 0.5, age: time.Hour, itemType: core.ContextTypeObservation},
			&testContextItem{id: "low", tokens: 10, relevance: 0.1, age: 5 * time.Hour, itemType: core.ContextTypeObservation},
		}
		if !reflect.DeepEqual(strategy.PrioritizeContext(items), wantItems) {
			t.Fatal("unexpected balanced prioritization")
		}
	})
}

func TestProfiledStrategyThresholdAndExpansionEdges(t *testing.T) {
	cases := []struct {
		name        string
		profile     StrategyProfile
		history     int
		want        bool
		result      *core.Result
		checkExpand bool
	}{
		{
			name:    "aggressive-at",
			profile: AggressiveProfile,
			history: 5,
			want:    false,
		},
		{
			name:    "aggressive-below",
			profile: AggressiveProfile,
			history: 4,
			want:    false,
		},
		{
			name:    "aggressive-above",
			profile: AggressiveProfile,
			history: 6,
			want:    true,
		},
		{
			name:    "conservative-below",
			profile: ConservativeProfile,
			history: 14,
			want:    false,
		},
		{
			name:    "conservative-at",
			profile: ConservativeProfile,
			history: 15,
			want:    false,
		},
		{
			name:    "conservative-above",
			profile: ConservativeProfile,
			history: 16,
			want:    true,
		},
		{
			name:    "balanced-below",
			profile: BalancedProfile,
			history: 9,
			want:    false,
		},
		{
			name:    "balanced-at",
			profile: BalancedProfile,
			history: 10,
			want:    false,
		},
		{
			name:    "balanced-above",
			profile: BalancedProfile,
			history: 11,
			want:    true,
		},
		{
			name:        "nil-result",
			profile:     AggressiveProfile,
			result:      nil,
			checkExpand: true,
			want:        false,
		},
		{
			name:        "success-no-trigger",
			profile:     AggressiveProfile,
			result:      &core.Result{Success: true},
			checkExpand: true,
			want:        false,
		},
		{
			name:        "error-type",
			profile:     AggressiveProfile,
			result:      &core.Result{Success: false, Data: map[string]any{"error_type": "file_not_found"}},
			checkExpand: true,
			want:        true,
		},
		{
			name:        "tool-use",
			profile:     ConservativeProfile,
			result:      &core.Result{Success: true, Data: map[string]any{"tool_used": "query_ast"}},
			checkExpand: true,
			want:        true,
		},
		{
			name:        "uncertainty",
			profile:     BalancedProfile,
			result:      &core.Result{Success: true, Data: map[string]any{"llm_output": "I am not sure"}},
			checkExpand: true,
			want:        true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			strategy := NewStrategyFromProfile(tc.profile)
			if tc.checkExpand {
				shared := core.NewSharedContext(core.NewContext(), core.NewContextBudget(2048), nil)
				if got := strategy.ShouldExpandContext(shared, tc.result); got != tc.want {
					t.Fatalf("unexpected expand result: got %v want %v", got, tc.want)
				}
				return
			}
			shared := core.NewSharedContext(core.NewContext(), core.NewContextBudget(2048), nil)
			for i := 0; i < tc.history; i++ {
				shared.AddInteraction("assistant", "history", nil)
			}
			if got := strategy.ShouldCompress(shared); got != tc.want {
				t.Fatalf("unexpected compress result: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestProfiledStrategyDetailLevelBoundaries(t *testing.T) {
	cases := []struct {
		name      string
		profile   StrategyProfile
		relevance float64
		want      DetailLevel
	}{
		{name: "aggressive-full", profile: AggressiveProfile, relevance: 0.95, want: DetailDetailed},
		{name: "aggressive-detailed-boundary", profile: AggressiveProfile, relevance: 0.9, want: DetailDetailed},
		{name: "aggressive-concise-boundary", profile: AggressiveProfile, relevance: 0.7, want: DetailConcise},
		{name: "aggressive-minimal-boundary", profile: AggressiveProfile, relevance: 0.5, want: DetailMinimal},
		{name: "aggressive-signature", profile: AggressiveProfile, relevance: 0.49, want: DetailSignatureOnly},
		{name: "conservative-full-boundary", profile: ConservativeProfile, relevance: 0.8, want: DetailFull},
		{name: "conservative-detailed-boundary", profile: ConservativeProfile, relevance: 0.5, want: DetailDetailed},
		{name: "conservative-concise", profile: ConservativeProfile, relevance: 0.49, want: DetailConcise},
		{name: "balanced-full-boundary", profile: BalancedProfile, relevance: 0.85, want: DetailFull},
		{name: "balanced-detailed-boundary", profile: BalancedProfile, relevance: 0.6, want: DetailDetailed},
		{name: "balanced-concise", profile: BalancedProfile, relevance: 0.59, want: DetailConcise},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewStrategyFromProfile(tc.profile).DetermineDetailLevel("file.go", tc.relevance); got != tc.want {
				t.Fatalf("unexpected detail level: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestNewStrategyFromProfileZeroValueIsSafe(t *testing.T) {
	strategy := NewStrategyFromProfile(StrategyProfile{})
	if strategy == nil {
		t.Fatal("expected strategy")
	}
	if got := strategy.ShouldCompress(nil); got {
		t.Fatal("expected zero-value profile to never compress")
	}
	if got := strategy.DetermineDetailLevel("file.go", 0.99); got != DetailSignatureOnly {
		t.Fatalf("unexpected zero-value detail level: %v", got)
	}
	if got := strategy.ShouldExpandContext(nil, nil); got {
		t.Fatal("expected zero-value profile to never expand")
	}
	if got := strategy.PrioritizeContext(nil); got != nil {
		t.Fatalf("unexpected prioritization of nil slice: %#v", got)
	}
}
