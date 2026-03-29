package euclotest

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo"
	"github.com/lexcodex/relurpify/testutil/euclotestutil"
)

func BenchmarkAgentExecuteModalWorkloads(b *testing.B) {
	benchmarks := []struct {
		name        string
		instruction string
		context     map[string]any
		seedVerify  bool
	}{
		{
			name:        "chat_ask_collect_context",
			instruction: "summarize the current implementation status",
			context:     map[string]any{"workspace": "/tmp/ws"},
			seedVerify:  true,
		},
		{
			name:        "chat_inspect_local_review",
			instruction: "inspect the authentication middleware and review the current flow",
			context:     map[string]any{"workspace": "/tmp/ws"},
			seedVerify:  true,
		},
		{
			name:        "chat_implement_direct_edit",
			instruction: "implement the requested change in pkg/foo",
			context:     map[string]any{"workspace": "/tmp/ws"},
			seedVerify:  true,
		},
		{
			name:        "debug_runtime_contract",
			instruction: "fix the failing test in pkg/foo and explain the root cause",
			context:     map[string]any{"workspace": "/tmp/ws", "mode": "debug"},
			seedVerify:  true,
		},
		{
			name:        "plan_backed_context_build",
			instruction: "plan the migration from the current API to the new auth flow",
			context:     map[string]any{"workspace": "/tmp/ws", "mode": "planning"},
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			agent := euclo.New(testutil.Env(b))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				state := core.NewContext()
				if bm.seedVerify {
					state.Set("pipeline.verify", map[string]any{
						"status":  "pass",
						"summary": "verification passed",
						"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
					})
				}
				result, err := agent.Execute(context.Background(), &core.Task{
					ID:          bm.name,
					Instruction: bm.instruction,
					Context:     bm.context,
				}, state)
				if err != nil {
					b.Fatalf("execute failed: %v", err)
				}
				if result == nil {
					b.Fatal("expected result")
				}
			}
		})
	}
}
