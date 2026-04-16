package runtime

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

// TestTier1_SingleKeywordMatch verifies exact match routes to correct capability
func TestTier1_SingleKeywordMatch(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
	}

	tests := []struct {
		instruction string
		modeID      string
		wantID      string
		wantSource  string
	}{
		{"explain how this works", "chat", euclorelurpic.CapabilityChatAsk, "keyword"},
		{"what is the purpose of this function", "chat", euclorelurpic.CapabilityChatAsk, "keyword"},
		{"implement a new feature", "chat", euclorelurpic.CapabilityChatImplement, "keyword"},
		{"fix the bug in counter.go", "chat", euclorelurpic.CapabilityChatImplement, "keyword"},
		{"inspect this code", "chat", euclorelurpic.CapabilityChatInspect, "keyword"},
		{"review this code", "chat", euclorelurpic.CapabilityChatInspect, "keyword"},
		{"investigate the failure", "debug", euclorelurpic.CapabilityDebugInvestigateRepair, "keyword"},
		{"root cause analysis needed", "debug", euclorelurpic.CapabilityDebugInvestigateRepair, "keyword"},
		{"explore the codebase", "planning", euclorelurpic.CapabilityArchaeologyExplore, "keyword"},
		{"find patterns in the code", "planning", euclorelurpic.CapabilityArchaeologyExplore, "keyword"},
	}

	for _, tc := range tests {
		t.Run(tc.instruction, func(t *testing.T) {
			result, err := classifier.Classify(context.Background(), tc.instruction, tc.modeID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Sequence) == 0 {
				t.Fatalf("expected sequence, got empty")
			}
			if result.Sequence[0] != tc.wantID {
				t.Errorf("want %q, got %q", tc.wantID, result.Sequence[0])
			}
			if result.Source != tc.wantSource {
				t.Errorf("want source %q, got %q", tc.wantSource, result.Source)
			}
		})
	}
}

// TestTier1_CompoundANDConnector verifies "inspect X and then fix it" produces AND sequence
func TestTier1_CompoundANDConnector(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
	}

	tests := []struct {
		instruction string
		modeID      string
		wantOp      string
		minSeqLen   int
	}{
		{"inspect the code and then fix it", "chat", "AND", 2},
		{"review the changes and then implement", "chat", "AND", 2},
		{"first analyze the problem then fix it", "chat", "AND", 2},
		{"explore the codebase followed by compile the plan", "planning", "AND", 2},
	}

	for _, tc := range tests {
		t.Run(tc.instruction, func(t *testing.T) {
			result, err := classifier.Classify(context.Background(), tc.instruction, tc.modeID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Operator != tc.wantOp {
				t.Errorf("want operator %q, got %q", tc.wantOp, result.Operator)
			}
			if len(result.Sequence) < tc.minSeqLen {
				t.Errorf("want at least %d capabilities, got %d", tc.minSeqLen, len(result.Sequence))
			}
		})
	}
}

// TestTier1_MultipleMatchHighestWins verifies unambiguous winner by MatchCount
func TestTier1_MultipleMatchHighestWins(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
	}

	// "explain what this code implements" matches both ask (explain) and implement (implements)
	// But "implements" is a stronger match for implement
	result, err := classifier.Classify(context.Background(), "explain what this code implements", "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get a single winner
	if len(result.Sequence) == 0 {
		t.Fatal("expected a capability, got none")
	}

	// The winner should be one of the matching capabilities
	validIDs := map[string]bool{
		euclorelurpic.CapabilityChatAsk:       true,
		euclorelurpic.CapabilityChatImplement: true,
	}
	if !validIDs[result.Sequence[0]] {
		t.Errorf("unexpected capability: %q", result.Sequence[0])
	}
}

// TestTier1_NoMatchProceeds verifies no match proceeds to fallback
func TestTier1_NoMatchProceeds(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
	}

	// Debug: check what keywords match
	matches := classifier.Registry.MatchByKeywords("do something", "chat", nil)
	t.Logf("Matches for 'do something': %v", matches)
	for _, m := range matches {
		t.Logf("  Match: ID=%s, Keywords=%v", m.ID, m.MatchedKeywords)
	}

	// Use a generic statement with no keyword matches
	result, err := classifier.Classify(context.Background(), "do something", "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Logf("Result: Sequence=%v, Source=%s, Meta=%s", result.Sequence, result.Source, result.Meta)

	// Should fall back to default for chat mode (ask)
	if len(result.Sequence) == 0 {
		t.Fatal("expected fallback capability, got none")
	}
	if result.Sequence[0] != euclorelurpic.CapabilityChatAsk {
		t.Errorf("want fallback %q, got %q", euclorelurpic.CapabilityChatAsk, result.Sequence[0])
	}
	if result.Source != "fallback" {
		t.Errorf("want source %q, got %q", "fallback", result.Source)
	}
}

// TestTier3_FallbackUsed verifies fallback capability is returned when no keyword match
func TestTier3_FallbackUsed(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
	}

	tests := []struct {
		modeID string
		wantID string
	}{
		{"chat", euclorelurpic.CapabilityChatAsk},
		{"debug", euclorelurpic.CapabilityDebugInvestigateRepair},
		{"planning", euclorelurpic.CapabilityArchaeologyExplore},
		// Note: "review" mode maps to chat.inspect in workunit.go but has no ModeFamilies: []string{"review"} capability
	}

	for _, tc := range tests {
		t.Run(tc.modeID, func(t *testing.T) {
			result, err := classifier.Classify(context.Background(), "xyz nonsense query", tc.modeID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Sequence) == 0 {
				t.Fatal("expected fallback capability, got none")
			}
			if result.Sequence[0] != tc.wantID {
				t.Errorf("want %q, got %q", tc.wantID, result.Sequence[0])
			}
			if result.Source != "fallback" {
				t.Errorf("want source %q, got %q", "fallback", result.Source)
			}
		})
	}
}

// TestTier3_NoFallbackRegistered verifies error when no DefaultForMode is set
func TestTier3_NoFallbackRegistered(t *testing.T) {
	// Create registry without any DefaultForMode set
	registry := euclorelurpic.NewRegistry()
	_ = registry.Register(euclorelurpic.Descriptor{
		ID:             "test:capability",
		ModeFamilies: []string{"testmode"},
		PrimaryCapable: true,
		Keywords:       []string{"test"},
		DefaultForMode: false, // explicitly not default
	})

	classifier := &CapabilityIntentClassifier{
		Registry: registry,
	}

	// Query with no keyword match in a mode with no fallback
	_, err := classifier.Classify(context.Background(), "random query", "testmode")
	if err == nil {
		t.Fatal("expected error for mode without fallback, got nil")
	}
}

// TestExtraKeywordsFromManifest verifies user-configured keyword triggers Tier 1 match
func TestExtraKeywordsFromManifest(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
		ExtraKeywords: map[string][]string{
			euclorelurpic.CapabilityChatAsk: {"clarify", "elaborate"},
		},
	}

	result, err := classifier.Classify(context.Background(), "clarify this code", "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Sequence) == 0 || result.Sequence[0] != euclorelurpic.CapabilityChatAsk {
		t.Errorf("want %q from extra keyword, got %v", euclorelurpic.CapabilityChatAsk, result.Sequence)
	}
	if result.Source != "keyword" {
		t.Errorf("want source %q, got %q", "keyword", result.Source)
	}
}

// TestDebugSimpleRepairKeywords verifies debug.repair.simple keyword matching
func TestDebugSimpleRepairKeywords(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
	}

	tests := []struct {
		instruction string
		wantID      string
	}{
		{"fix this bug", euclorelurpic.CapabilityDebugRepairSimple},
		{"apply a fix", euclorelurpic.CapabilityDebugRepairSimple},
		{"quick fix needed", euclorelurpic.CapabilityDebugRepairSimple},
		{"off-by-one error", euclorelurpic.CapabilityDebugRepairSimple},
		{"returns wrong value", euclorelurpic.CapabilityDebugRepairSimple},
		{"investigate the crash", euclorelurpic.CapabilityDebugInvestigateRepair},
		{"root cause analysis", euclorelurpic.CapabilityDebugInvestigateRepair},
	}

	for _, tc := range tests {
		t.Run(tc.instruction, func(t *testing.T) {
			result, err := classifier.Classify(context.Background(), tc.instruction, "debug")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Sequence) == 0 {
				t.Fatal("expected capability, got none")
			}
			if result.Sequence[0] != tc.wantID {
				t.Errorf("want %q, got %q", tc.wantID, result.Sequence[0])
			}
		})
	}
}

// stubLanguageModel is a test double for core.LanguageModel
type stubLanguageModel struct {
	response string
	err      error
}

func (s *stubLanguageModel) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &core.LLMResponse{Text: s.response}, nil
}

func (s *stubLanguageModel) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubLanguageModel) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubLanguageModel) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// TestTier2_SingleCapabilityResponse verifies LLM returning single capability ID
func TestTier2_SingleCapabilityResponse(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
		Model: &stubLanguageModel{
			response: "euclo:chat.ask",
		},
	}

	// Domain-specific phrasing with no keyword match - should go to Tier 2
	result, err := classifier.Classify(context.Background(), "elucidate the purpose of this function", "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Sequence) == 0 || result.Sequence[0] != euclorelurpic.CapabilityChatAsk {
		t.Errorf("want %q, got %v", euclorelurpic.CapabilityChatAsk, result.Sequence)
	}
	if result.Source != "llm_semantic" {
		t.Errorf("want source %q, got %q", "llm_semantic", result.Source)
	}
	if result.Operator != "AND" {
		t.Errorf("want operator %q, got %q", "AND", result.Operator)
	}
}

// TestTier2_AND_Response verifies LLM returning AND sequence
func TestTier2_AND_Response(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
		Model: &stubLanguageModel{
			response: "euclo:chat.inspect AND euclo:chat.implement",
		},
	}

	result, err := classifier.Classify(context.Background(), "some query with no keyword matches", "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Sequence) != 2 {
		t.Errorf("want 2 capabilities, got %d", len(result.Sequence))
	}
	if result.Sequence[0] != euclorelurpic.CapabilityChatInspect {
		t.Errorf("want first %q, got %q", euclorelurpic.CapabilityChatInspect, result.Sequence[0])
	}
	if result.Sequence[1] != euclorelurpic.CapabilityChatImplement {
		t.Errorf("want second %q, got %q", euclorelurpic.CapabilityChatImplement, result.Sequence[1])
	}
	if result.Operator != "AND" {
		t.Errorf("want operator %q, got %q", "AND", result.Operator)
	}
	if result.Source != "llm_semantic" {
		t.Errorf("want source %q, got %q", "llm_semantic", result.Source)
	}
}

// TestTier2_OR_Response verifies LLM returning OR sequence
func TestTier2_OR_Response(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
		Model: &stubLanguageModel{
			response: "euclo:debug.investigate-repair OR euclo:debug.repair.simple",
		},
	}

	// Use a query with no keyword matches to force Tier 2
	result, err := classifier.Classify(context.Background(), "xyz qwerty 12345", "debug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Sequence) != 2 {
		t.Errorf("want 2 capabilities, got %d", len(result.Sequence))
	}
	if result.Operator != "OR" {
		t.Errorf("want operator %q, got %q", "OR", result.Operator)
	}
	if result.Source != "llm_semantic" {
		t.Errorf("want source %q, got %q", "llm_semantic", result.Source)
	}
}

// TestTier2_NoneResponse_FallsBackToTier3 verifies "none" response falls back
func TestTier2_NoneResponse_FallsBackToTier3(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
		Model: &stubLanguageModel{
			response: "none",
		},
	}

	// Use a query with no keyword matches to force Tier 2
	result, err := classifier.Classify(context.Background(), "xyz qwerty 12345", "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fall back to chat.ask
	if len(result.Sequence) == 0 || result.Sequence[0] != euclorelurpic.CapabilityChatAsk {
		t.Errorf("want fallback %q, got %v", euclorelurpic.CapabilityChatAsk, result.Sequence)
	}
	if result.Source != "fallback" {
		t.Errorf("want source %q, got %q", "fallback", result.Source)
	}
}

// TestTier2_LLMErrorSurfaces verifies LLM errors are returned (not swallowed)
func TestTier2_LLMErrorSurfaces(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
		Model: &stubLanguageModel{
			err: fmt.Errorf("llm transport error: connection refused"),
		},
	}

	_, err := classifier.Classify(context.Background(), "something with no keyword match", "chat")
	if err == nil {
		t.Fatal("expected LLM error to surface, got nil")
	}
	if !strings.Contains(err.Error(), "llm classification query failed") {
		t.Errorf("expected wrapped llm error, got: %v", err)
	}
}

// TestParseCapabilitySequenceResponse_AND verifies AND parsing
func TestParseCapabilitySequenceResponse_AND(t *testing.T) {
	validIDs := map[string]bool{
		"euclo:chat.inspect":   true,
		"euclo:chat.implement": true,
	}

	ids, operator, err := parseCapabilitySequenceResponse("euclo:chat.inspect AND euclo:chat.implement", validIDs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("want 2 ids, got %d", len(ids))
	}
	if operator != "AND" {
		t.Errorf("want operator AND, got %q", operator)
	}
}

// TestParseCapabilitySequenceResponse_OR verifies OR parsing
func TestParseCapabilitySequenceResponse_OR(t *testing.T) {
	validIDs := map[string]bool{
		"euclo:debug.investigate-repair": true,
		"euclo:debug.repair.simple":      true,
	}

	ids, operator, err := parseCapabilitySequenceResponse("euclo:debug.investigate-repair OR euclo:debug.repair.simple", validIDs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("want 2 ids, got %d", len(ids))
	}
	if operator != "OR" {
		t.Errorf("want operator OR, got %q", operator)
	}
}

// TestParseCapabilitySequenceResponse_None verifies "none" parsing
func TestParseCapabilitySequenceResponse_None(t *testing.T) {
	validIDs := map[string]bool{"euclo:chat.ask": true}

	ids, operator, err := parseCapabilitySequenceResponse("none", validIDs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("want 0 ids for 'none', got %d", len(ids))
	}
	if operator != "AND" {
		t.Errorf("want operator AND for 'none', got %q", operator)
	}
}

// TestParseCapabilitySequenceResponse_InvalidID verifies error on unknown capability
func TestParseCapabilitySequenceResponse_InvalidID(t *testing.T) {
	validIDs := map[string]bool{"euclo:chat.ask": true}

	_, _, err := parseCapabilitySequenceResponse("euclo:invalid.capability", validIDs)
	if err == nil {
		t.Fatal("expected error for invalid capability ID, got nil")
	}
}

// TestCompoundOrConnector verifies OR connector detection
func TestCompoundOrConnector(t *testing.T) {
	tests := []struct {
		instruction string
		wantOr      bool
	}{
		{"fix this or that", true},
		{"either implement or fix", true},
		{"whether to refactor or rewrite", true},
		{"pick one of these options", true},
		{"which approach should I use", true},
		{"fix this and that", false},
		{"just fix it", false},
	}

	for _, tc := range tests {
		t.Run(tc.instruction, func(t *testing.T) {
			got := hasCompoundOrConnector(tc.instruction)
			if got != tc.wantOr {
				t.Errorf("hasCompoundOrConnector(%q) = %v, want %v", tc.instruction, got, tc.wantOr)
			}
		})
	}
}

// TestBaselinePromptRouting validates that baseline test prompts route to expected capabilities
func TestBaselinePromptRouting(t *testing.T) {
	classifier := &CapabilityIntentClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
	}

	tests := []struct {
		name        string
		instruction string
		modeID      string
		wantID      string
	}{
		{
			name:        "debug_investigate_baseline",
			instruction: "Identify the likely root cause and where it is localized. Do not modify files.",
			modeID:      "debug",
			wantID:      euclorelurpic.CapabilityDebugInvestigateRepair,
		},
		{
			name:        "debug_simple_repair_baseline",
			instruction: "fix the bug in counter.go",
			modeID:      "debug",
			wantID:      euclorelurpic.CapabilityDebugRepairSimple,
		},
		{
			name:        "chat_ask_baseline",
			instruction: "Explain what the User type represents.",
			modeID:      "chat",
			wantID:      euclorelurpic.CapabilityChatAsk,
		},
		{
			name:        "chat_inspect_baseline",
			instruction: "Compare the behavior of MemoryStore",
			modeID:      "chat",
			wantID:      euclorelurpic.CapabilityChatInspect,
		},
		{
			name:        "chat_implement_baseline",
			instruction: "implement a new feature",
			modeID:      "chat",
			wantID:      euclorelurpic.CapabilityChatImplement,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := classifier.Classify(context.Background(), tc.instruction, tc.modeID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Sequence) == 0 {
				t.Fatal("expected sequence, got empty")
			}
			if result.Sequence[0] != tc.wantID {
				t.Errorf("want %q, got %q", tc.wantID, result.Sequence[0])
			}
		})
	}
}
