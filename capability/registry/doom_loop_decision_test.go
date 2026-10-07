package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type recordingTelemetry struct {
	events []fwtelemetry.Event
}

func (s *recordingTelemetry) Emit(ev fwtelemetry.Event) {
	s.events = append(s.events, ev)
}

// realDetectorDoomError drives a real DoomLoopDetector to its identical-call
// threshold so the test covers the production detection code path.
func realDetectorDoomError() *DoomLoopError {
	detector := NewDoomLoopDetector(DefaultDoomLoopConfig())
	desc := descriptor.CapabilityDescriptor{ID: "doom.cap", Name: "doom"}
	args := map[string]any{"path": "/tmp/x"}
	for i := 0; i < DefaultDoomLoopConfig().IdenticalCallThreshold+1; i++ {
		if err := detector.Check(desc, args); err != nil {
			var doomErr *DoomLoopError
			if errors.As(err, &doomErr) {
				return doomErr
			}
		}
	}
	return nil
}

func TestRegistry_EmitsDoomLoopDecisionEvent(t *testing.T) {
	sink := &recordingTelemetry{}
	reg := NewRegistry()
	reg.UseTelemetry(sink)

	doomErr := realDetectorDoomError()
	require.NotNil(t, doomErr, "detector must reach its identical-call threshold in this test")
	require.Equal(t, DoomLoopIdenticalCall, doomErr.Kind)

	reg.emitDoomLoopDetected(context.Background(), descriptor.CapabilityDescriptor{ID: "doom.cap"}, *doomErr)

	require.Len(t, sink.events, 1)
	ev := sink.events[0]
	require.Equal(t, fwtelemetry.EventDoomLoopDetected, ev.Type)
	require.Equal(t, "identical_call", ev.Metadata["kind"])
	require.Equal(t, "doom.cap", ev.Metadata["target"])
	require.Equal(t, DefaultDoomLoopConfig().IdenticalCallThreshold, ev.Metadata["calls"])
	// NFR-7: raw evidence is args-derived and may carry secrets — it stays
	// out of telemetry.
	require.NotContains(t, ev.Metadata, "evidence")
}

func TestRegistry_DoomLoopEmissionWithoutTelemetryIsNoop(t *testing.T) {
	reg := NewRegistry()
	doomErr := realDetectorDoomError()
	require.NotNil(t, doomErr)
	require.NotPanics(t, func() {
		reg.emitDoomLoopDetected(context.Background(), descriptor.CapabilityDescriptor{ID: "doom.cap"}, *doomErr)
	})
}
