package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"codeburg.org/lexbit/relurpify/telemetry"
)

const maxExecEventsPerBatch = 64

// ExecEventSink is an optional interface an AgentSurface can implement to
// receive execution telemetry events for live projection.
type ExecEventSink interface {
	ApplyExecEvent(ev any)
}

// listenExecEvents drains the execution event channel in batches. It blocks
// on the first event, then non-blocking drains up to maxExecEventsPerBatch
// additional events before returning a single batch message.
func listenExecEvents(ch <-chan telemetry.Event, epoch uint64) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		batch := make([]telemetry.Event, 0, maxExecEventsPerBatch)
		batch = append(batch, ev)
		for i := 0; i < maxExecEventsPerBatch-1; i++ {
			select {
			case extra, ok := <-ch:
				if !ok {
					return ExecEventBatchMsg{Events: batch, epoch: epoch, source: ch}
				}
				batch = append(batch, extra)
			default:
				return ExecEventBatchMsg{Events: batch, epoch: epoch, source: ch}
			}
		}
		return ExecEventBatchMsg{Events: batch, epoch: epoch, source: ch}
	}
}

// handleExecEventBatch applies a batch of execution telemetry events to the
// active surface's ExecEventSink, then re-arms the listener.
func (m RootModel) handleExecEventBatch(msg ExecEventBatchMsg) (RootModel, tea.Cmd) {
	if msg.epoch != m.observationEpoch || msg.source != m.execEventCh {
		return m, nil
	}
	m.activity.apply(msg.Events)
	sink, ok := m.activeSurface.(ExecEventSink)
	if ok {
		for _, ev := range msg.Events {
			sink.ApplyExecEvent(ev)
		}
		m.refreshCircuit()
	}
	return m, listenExecEvents(m.execEventCh, m.observationEpoch)
}
