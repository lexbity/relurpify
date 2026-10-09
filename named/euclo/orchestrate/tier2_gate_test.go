package orchestrate

import (
	"context"
	"errors"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// tier2TestModel drives the bounded disambiguator deterministically.
type tier2TestModel struct {
	mu    sync.Mutex
	text  string
	err   error
	calls int
}

func (m *tier2TestModel) Generate(_ context.Context, _ string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return &model.LLMResponse{Text: m.text}, nil
}

func (m *tier2TestModel) GenerateStream(_ context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *tier2TestModel) Chat(_ context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: m.text}, m.err
}

func (m *tier2TestModel) ChatWithTools(_ context.Context, _ []model.Message, _ []model.LLMToolSpec, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: m.text}, m.err
}

func (m *tier2TestModel) invocationCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func tier2Report(candidates []CandidateRouteInfo) *DryRunReport {
	return &DryRunReport{
		Request:    RouteRequest{},
		Candidates: candidates,
		DecidedBy:  decidedByScore,
	}
}

// TestTier2GateStrongMatchSkipsModel is AC-9's first half: the model is never
// consulted when the deterministic winner is strong (top1 ≥ floor and gap > band).
func TestTier2GateStrongMatchSkipsModel(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableCapability("euclo:cap.strong", 80, nil),
		availableCapability("euclo:cap.weak", 30, nil),
	}
	report := tier2Report(candidates)
	selected := candidates[0]
	m := &tier2TestModel{}

	got, decidedBy, info := applyTier2Gate(context.Background(), nil, RouteRequest{}, report, selected, report.DecidedBy, SelectionDeps{Tier2Model: m})
	if m.invocationCount() != 0 {
		t.Fatalf("strong match invoked the model %d times", m.invocationCount())
	}
	if info.Used {
		t.Fatalf("strong match recorded a tier-2 attempt: %+v", info)
	}
	if got.RouteID != selected.RouteID || decidedBy != report.DecidedBy {
		t.Fatalf("strong match changed selection to %q/%q", got.RouteID, decidedBy)
	}
}

// TestTier2GateTieAdoptsModelChoice: in a tie band the model is consulted once
// and its in-set, high-confidence answer is adopted.
func TestTier2GateTieAdoptsModelChoice(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableCapability("euclo:cap.a", 70, nil),
		availableCapability("euclo:cap.b", 62, nil),
	}
	report := tier2Report(candidates)
	selected := candidates[0]
	m := &tier2TestModel{text: `{"id": "euclo:cap.b", "confidence": 0.95}`}

	got, decidedBy, info := applyTier2Gate(context.Background(), nil, RouteRequest{}, report, selected, report.DecidedBy, SelectionDeps{Tier2Model: m})
	if m.invocationCount() != 1 {
		t.Fatalf("tie triggered %d model calls, want 1", m.invocationCount())
	}
	if info.Outcome != "applied" || !info.Used {
		t.Fatalf("tier-2 info = %+v, want used/applied", info)
	}
	if got.RouteID != "euclo:cap.b" {
		t.Fatalf("adopted route = %q, want the model-answered euclo:cap.b", got.RouteID)
	}
	if decidedBy != decidedByTier2 {
		t.Fatalf("decidedBy = %q, want tier2", decidedBy)
	}
}

// TestTier2GateOutOfSetKeepsDeterministicWinner: the model can never select
// outside the deterministic candidate set.
func TestTier2GateOutOfSetKeepsDeterministicWinner(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableCapability("euclo:cap.a", 70, nil),
		availableCapability("euclo:cap.b", 62, nil),
	}
	report := tier2Report(candidates)
	selected := candidates[0]
	m := &tier2TestModel{text: `{"id": "euclo:cap.somewhere_else", "confidence": 0.99}`}

	got, _, info := applyTier2Gate(context.Background(), nil, RouteRequest{}, report, selected, report.DecidedBy, SelectionDeps{Tier2Model: m})
	if info.Outcome != "rejected" {
		t.Fatalf("outcome = %q, want rejected", info.Outcome)
	}
	if got.RouteID != selected.RouteID {
		t.Fatalf("selection changed to %q, want deterministic winner %q", got.RouteID, selected.RouteID)
	}
}

// TestTier2GateLowConfidenceKeepsDeterministicWinner: an in-set id below the
// confidence floor is recorded as low_confidence and does not win.
func TestTier2GateLowConfidenceKeepsDeterministicWinner(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableCapability("euclo:cap.a", 70, nil),
		availableCapability("euclo:cap.b", 62, nil),
	}
	report := tier2Report(candidates)
	selected := candidates[0]
	m := &tier2TestModel{text: `{"id": "euclo:cap.b", "confidence": 0.3}`}

	got, _, info := applyTier2Gate(context.Background(), nil, RouteRequest{}, report, selected, report.DecidedBy, SelectionDeps{Tier2Model: m})
	if info.Outcome != "low_confidence" {
		t.Fatalf("outcome = %q, want low_confidence", info.Outcome)
	}
	if got.RouteID != selected.RouteID {
		t.Fatalf("low-confidence answer changed the selection to %q", got.RouteID)
	}
}

// TestTier2GateUnparseableKeepsDeterministicWinner covers a garbage response.
func TestTier2GateUnparseableKeepsDeterministicWinner(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableCapability("euclo:cap.a", 70, nil),
		availableCapability("euclo:cap.b", 62, nil),
	}
	report := tier2Report(candidates)
	selected := candidates[0]
	m := &tier2TestModel{text: "the answer is obviously capability a"}

	got, _, info := applyTier2Gate(context.Background(), nil, RouteRequest{}, report, selected, report.DecidedBy, SelectionDeps{Tier2Model: m})
	if info.Outcome != "unparseable" {
		t.Fatalf("outcome = %q, want unparseable", info.Outcome)
	}
	if got.RouteID != selected.RouteID {
		t.Fatalf("unparseable answer changed the selection to %q", got.RouteID)
	}
}

// TestTier2GateUnavailableEmitsEventAndKeepsWinner covers the model error path:
// route.tier2_unavailable fires, selection proceeds with the deterministic winner.
func TestTier2GateUnavailableEmitsEventAndKeepsWinner(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableCapability("euclo:cap.a", 70, nil),
		availableCapability("euclo:cap.b", 62, nil),
	}
	report := tier2Report(candidates)
	selected := candidates[0]
	m := &tier2TestModel{err: errors.New("provider unreachable")}
	sink := &telemetrySink{}
	ctx := telemetry.WithTelemetry(context.Background(), sink)

	got, _, info := applyTier2Gate(ctx, nil, RouteRequest{}, report, selected, report.DecidedBy, SelectionDeps{Tier2Model: m})
	if info.Outcome != "unavailable" || !info.Used {
		t.Fatalf("tier-2 info = %+v, want used/unavailable", info)
	}
	if got.RouteID != selected.RouteID {
		t.Fatalf("selection changed to %q despite no model", got.RouteID)
	}
	found := false
	for _, event := range sink.snapshot() {
		if event.Type == telemetry.EventType(reporting.EventTypeRouteTier2Unavailable) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected route.tier2_unavailable telemetry")
	}
}

// TestTier2GateNeverAdoptsOutsideTopK: a candidate ranked beyond the top-K refs
// cannot win even when the model names it.
func TestTier2GateNeverAdoptsOutsideTopK(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableCapability("euclo:cap.c0", 70, nil),
		availableCapability("euclo:cap.c1", 68, nil),
		availableCapability("euclo:cap.c2", 66, nil),
		availableCapability("euclo:cap.c3", 64, nil),
		availableCapability("euclo:cap.c4", 62, nil),
		availableCapability("euclo:cap.c5", 60, nil),
	}
	report := tier2Report(candidates)
	selected := candidates[0]
	m := &tier2TestModel{text: `{"id": "euclo:cap.c5", "confidence": 0.99}`}

	got, _, info := applyTier2Gate(context.Background(), nil, RouteRequest{}, report, selected, report.DecidedBy, SelectionDeps{Tier2Model: m})
	if info.Outcome != "rejected" {
		t.Fatalf("6th-ranked answer outcome = %q, want rejected (outside top-5)", info.Outcome)
	}
	if got.RouteID != selected.RouteID {
		t.Fatalf("6th-ranked answer changed the selection to %q", got.RouteID)
	}
}

// TestTier2GateNoModelRecordsUnavailableWithoutBlocking: an unconfigured model
// on a weak/tie match is reported, never blocking selection.
func TestTier2GateNoModelRecordsUnavailableWithoutBlocking(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableCapability("euclo:cap.a", 70, nil),
		availableCapability("euclo:cap.b", 62, nil),
	}
	report := tier2Report(candidates)
	selected := candidates[0]

	got, decidedBy, info := applyTier2Gate(context.Background(), nil, RouteRequest{}, report, selected, report.DecidedBy, SelectionDeps{})
	if info.Outcome != "unavailable" || !info.Used {
		t.Fatalf("nil model info = %+v, want used/unavailable", info)
	}
	if got.RouteID != selected.RouteID || decidedBy != report.DecidedBy {
		t.Fatalf("nil model changed the selection to %q/%q", got.RouteID, decidedBy)
	}
}

// TestTier2GateSkipsExplicitAndDefault ensures the gate is never consulted for
// rank-1 explicit requests or the default-recipe degradation.
func TestTier2GateSkipsExplicitAndDefault(t *testing.T) {
	explicit := tier2Report([]CandidateRouteInfo{availableCapability("euclo:cap.a", 1000, nil)})
	explicit.Request = RouteRequest{CapabilityID: "euclo:cap.a"}
	explicit.DecidedBy = decidedByExplicit
	m := &tier2TestModel{text: `{"id": "x", "confidence": 0.9}`}
	if got, _, info := applyTier2Gate(context.Background(), nil, RouteRequest{}, explicit, explicit.Candidates[0], explicit.DecidedBy, SelectionDeps{Tier2Model: m}); info.Used || m.invocationCount() != 0 || got.RouteID != "euclo:cap.a" {
		t.Fatalf("explicit request consulted tier-2: info=%+v calls=%d", info, m.invocationCount())
	}

	m2 := &tier2TestModel{text: `{"id": "x", "confidence": 0.9}`}
	fallback := tier2Report([]CandidateRouteInfo{availableCapability("euclo:cap.fallback", 0, nil)})
	fallback.DecidedBy = decidedByDefaultRecipe
	if got, _, info := applyTier2Gate(context.Background(), nil, RouteRequest{}, fallback, fallback.Candidates[0], fallback.DecidedBy, SelectionDeps{Tier2Model: m2}); info.Used || m2.invocationCount() != 0 || got.RouteID != "euclo:cap.fallback" {
		t.Fatalf("default-recipe fallback consulted tier-2: info=%+v calls=%d", info, m2.invocationCount())
	}
}
