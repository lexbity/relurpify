package tui

import (
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/telemetry"
	"github.com/stretchr/testify/require"
)

func TestV2ObservationsRejectPreviousWorkspaceAndSubscription(t *testing.T) {
	m, _ := newFocusTestModel()
	m.observationEpoch = 2
	oldChannel, newChannel := make(chan telemetry.Event, 1), make(chan telemetry.Event, 1)
	released := 0
	next, _ := m.Update(execEventsSubscribedMsg{epoch: 1, ch: oldChannel, unsub: func() { released++ }})
	m = next.(RootModel)
	require.Equal(t, 1, released)
	require.Nil(t, m.execEventCh)
	next, _ = m.Update(execEventsSubscribedMsg{epoch: 2, ch: newChannel, unsub: func() { released++ }})
	m = next.(RootModel)
	require.Equal(t, (<-chan telemetry.Event)(newChannel), m.execEventCh)
	stale := ExecEventBatchMsg{epoch: 1, source: oldChannel, Events: []telemetry.Event{{Type: telemetry.EventLLMPrompt}}}
	next, cmd := m.Update(stale)
	m = next.(RootModel)
	require.Nil(t, cmd)
	require.Empty(t, m.activity.rows)
	stale.epoch = 2 // Same workspace, superseded subscription still cannot publish.
	next, cmd = m.Update(stale)
	m = next.(RootModel)
	require.Nil(t, cmd)
	require.Empty(t, m.activity.rows)
	current := ExecEventBatchMsg{epoch: 2, source: newChannel, Events: []telemetry.Event{{Type: telemetry.EventCompilerCompleted}}}
	next, cmd = m.Update(current)
	m = next.(RootModel)
	require.NotNil(t, cmd)
	require.Len(t, m.activity.rows, 1)
	m.cleanup()
	require.Equal(t, 2, released)
}

func TestV2AttentionRejectsStaleWorkspaceAndResolutionFailure(t *testing.T) {
	m, _ := newFocusTestModel()
	m.observationEpoch = 2
	req := &authorization.PermissionRequest{ID: "old-request"}
	next, cmd := m.Update(hitlEventMsg{epoch: 1, event: authorization.HITLEvent{Type: authorization.HITLEventRequested, Request: req}})
	m = next.(RootModel)
	require.Nil(t, cmd)
	require.Zero(t, m.notifQ.Len())
	m.notifQ.PushHITL(req)
	next, cmd = m.Update(hitlResolvedMsg{requestID: req.ID, err: errors.New("store unavailable")})
	m = next.(RootModel)
	require.Nil(t, cmd, "resolution created a second event listener")
	require.Equal(t, 1, m.notifQ.Len(), "failed resolution discarded unresolved attention")
	next, cmd = m.Update(hitlResolvedMsg{requestID: req.ID, approved: true})
	m = next.(RootModel)
	require.Nil(t, cmd)
	require.Zero(t, m.notifQ.Len())
}
