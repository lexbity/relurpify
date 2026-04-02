package agenttestscenario

import (
	"fmt"
	"reflect"
	"testing"

	chaintelemetry "github.com/lexcodex/relurpify/agents/chainer/telemetry"
	"github.com/lexcodex/relurpify/framework/core"
)

func RequireModelExhausted(tb testing.TB, f *Fixture) {
	tb.Helper()
	if f == nil || f.Model == nil {
		tb.Fatalf("fixture model unavailable")
	}
	f.Model.AssertExhausted(tb)
}

func RequireExecutorCallCount(tb testing.TB, f *Fixture, want int) {
	tb.Helper()
	if f == nil || f.Exec == nil {
		tb.Fatalf("fixture executor unavailable")
	}
	if f.Exec.Calls != want {
		tb.Fatalf("executor call count = %d, want %d", f.Exec.Calls, want)
	}
}

func RequireTelemetryEventKind(tb testing.TB, f *Fixture, kind string) {
	tb.Helper()
	if f == nil {
		tb.Fatalf("fixture unavailable")
	}
	for _, event := range f.Telemetry.Events {
		if string(event.Type) == kind {
			return
		}
	}
	if f.Events != nil {
		for _, event := range f.Events.AllEvents("") {
			if string(event.Kind) == kind {
				return
			}
		}
	}
	tb.Fatalf("telemetry event kind %q not recorded", kind)
}

func RequireChainerEventKind(tb testing.TB, recorder *chaintelemetry.EventRecorder, taskID string, kind chaintelemetry.ChainerEventKind) {
	tb.Helper()
	if recorder == nil {
		tb.Fatalf("event recorder unavailable")
	}
	if len(recorder.EventsByKind(taskID, kind)) == 0 {
		tb.Fatalf("chainer event kind %q not recorded for task %q", kind, taskID)
	}
}

func RequireResultSuccess(tb testing.TB, result *core.Result) {
	tb.Helper()
	if result == nil {
		tb.Fatal("result unexpectedly nil")
	}
	if !result.Success {
		tb.Fatalf("expected success result, got %+v", result)
	}
}

func RequireContextKey(tb testing.TB, state *core.Context, key string, want interface{}) {
	tb.Helper()
	if state == nil {
		tb.Fatalf("context unavailable")
	}
	got, ok := state.Get(key)
	if !ok {
		tb.Fatalf("missing context key %q", key)
	}
	if !reflect.DeepEqual(got, want) {
		tb.Fatalf("context[%s] = %v, want %v", key, got, want)
	}
}

func Require(tb testing.TB, ok bool, format string, args ...any) {
	tb.Helper()
	if !ok {
		tb.Fatalf(format, args...)
	}
}

func Message(label string, value any) string {
	return fmt.Sprintf("%s=%v", label, value)
}
