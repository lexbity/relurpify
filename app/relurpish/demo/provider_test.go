package demo_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestV2ProviderReviewSaveActivateAndRetainedDraft(t *testing.T) {
	m, session := model(t, "quiet")
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("keep Euclo request")})
	m = command(m, "model-provider")
	require.Contains(t, m.View(), "Settings / Provider")
	m = press(m, tea.KeyDown, tea.KeyDown, tea.KeyEnter)
	m = press(m, tea.KeyCtrlA, tea.KeyCtrlK)
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Paste: true, Runes: []rune("demo-coder")})
	m = press(m, tea.KeyEnter)
	// Field staging never changes either saved or active state.
	before, err := session.ProviderSettings(t.Context())
	require.NoError(t, err)
	require.Equal(t, "demo-small", before.Saved.Model)
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	require.Contains(t, m.View(), "Review save")
	m = press(m, tea.KeyEsc)
	after, err := session.ProviderSettings(t.Context())
	require.NoError(t, err)
	require.Equal(t, before, after)
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = press(m, tea.KeyEnter)
	after, err = session.ProviderSettings(t.Context())
	require.NoError(t, err)
	require.Equal(t, "demo-coder", after.Saved.Model)
	require.Equal(t, before.ActiveSummary, after.ActiveSummary)
	require.Equal(t, before.ActiveRevision, after.ActiveRevision)
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	require.Contains(t, m.View(), "Review activate")
	m = press(m, tea.KeyEnter)
	active, err := session.ProviderSettings(t.Context())
	require.NoError(t, err)
	require.Equal(t, after.Revision, active.ActiveRevision)
	require.Contains(t, active.ActiveSummary, "demo-coder")
	m = press(m, tea.KeyEsc)
	require.True(t, m.InteractionState().Focus.InInput())
	require.Equal(t, "keep Euclo request", m.InteractionState().Draft)
}

func TestV2ProviderActivationFailureRetainsLastGoodState(t *testing.T) {
	m, session := model(t, "provider-failure")
	m = command(m, "model-provider")
	m = command(m, "model", "demo-coder")
	m = command(m, "save-model")
	m = press(m, tea.KeyEnter)
	saved, err := session.ProviderSettings(t.Context())
	require.NoError(t, err)
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = press(m, tea.KeyEnter)
	require.Contains(t, m.View(), "staged composition failed")
	retained, err := session.ProviderSettings(t.Context())
	require.NoError(t, err)
	require.Equal(t, saved.ActiveRevision, retained.ActiveRevision)
	require.NotEqual(t, retained.Revision, retained.ActiveRevision)
}

func TestV2DemoActivityIsLabeledAndDoesNotDuplicate(t *testing.T) {
	m, session := model(t, "quiet")
	m = command(m, "activity", "calls")
	require.Contains(t, m.View(), "DEMO")
	// Constructor's initial snapshot must not skip observations or duplicate them.
	require.Contains(t, m.View(), "llm_prompt")
	m = press(m, tea.KeyEnter)
	require.Contains(t, m.View(), "DEMO: simulated telemetry fixture")
	require.Contains(t, m.View(), "not captured")
	snapshot := session.Current()
	snapshot.Observations[0].Metadata["token_estimate"] = "tampered"
	require.EqualValues(t, 2400, session.Current().Observations[0].Metadata["token_estimate"])
}
