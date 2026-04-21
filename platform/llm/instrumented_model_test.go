package llm

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type profileAwareStubModel struct {
	profile *ModelProfile
}

func (m *profileAwareStubModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "ok"}, nil
}

func (m *profileAwareStubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *profileAwareStubModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "ok"}, nil
}

func (m *profileAwareStubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "ok"}, nil
}

func (m *profileAwareStubModel) SetProfile(profile *ModelProfile) {
	m.profile = profile
}

func (m *profileAwareStubModel) ToolRepairStrategy() string {
	if m.profile == nil {
		return "heuristic-only"
	}
	return m.profile.Repair.Strategy
}

func (m *profileAwareStubModel) MaxToolsPerCall() int {
	if m.profile == nil {
		return 0
	}
	return m.profile.ToolCalling.MaxToolsPerCall
}

func (m *profileAwareStubModel) UsesNativeToolCalling() bool {
	return m.profile != nil && m.profile.ToolCalling.NativeAPI
}

func TestInstrumentedModel_ProxiesProfileAwareBehavior(t *testing.T) {
	inner := &profileAwareStubModel{}
	model := NewInstrumentedModel(inner, nil, false)

	profile := &ModelProfile{}
	profile.ToolCalling.NativeAPI = true
	profile.ToolCalling.MaxToolsPerCall = 2
	profile.Repair.Strategy = "llm"

	model.SetProfile(profile)

	require.NotNil(t, inner.profile)
	require.True(t, model.UsesNativeToolCalling())
	require.Equal(t, "llm", model.ToolRepairStrategy())
	require.Equal(t, 2, model.MaxToolsPerCall())

	_, ok := any(model).(core.ProfiledModel)
	require.True(t, ok)
}
