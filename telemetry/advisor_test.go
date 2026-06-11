package telemetry

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextBudgetAdvisorRecordThenQuery(t *testing.T) {
	advisor := &ContextBudgetAdvisor{ModelContextSize: 8000, ReservedOutputTokens: 1024}
	advisor.RecordCall(TokenUsage{PromptTokens: 1024, CompletionTokens: 128, TotalTokens: 1152})
	advisor.RecordCall(TokenUsage{PromptTokens: 1000, Estimated: true})
	advisor.RecordCall(TokenUsage{PromptTokens: 1200})
	require.True(t, advisor.ShouldReset())
	advisor.Reset()
	require.Equal(t, 2048-512, advisor.AvailableCompilationBudget())
}

func TestContextBudgetAdvisor_ConcurrentRecordCall(t *testing.T) {
	advisor := &ContextBudgetAdvisor{ModelContextSize: 4096}
	const workers = 32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			advisor.RecordCall(TokenUsage{PromptTokens: 10, CompletionTokens: 5})
		}()
	}
	wg.Wait()
	require.Equal(t, 4096-workers*10-512, advisor.AvailableCompilationBudget())
	snap := advisor.Snapshot()
	require.Equal(t, workers, snap.CallCount)
	require.Equal(t, 4096, snap.ModelContextSize)
}

func TestContextBudgetAdvisor_ContextValueRoundTrip(t *testing.T) {
	advisor := &ContextBudgetAdvisor{ModelContextSize: 1024}
	ctx := WithAdvisor(context.TODO(), advisor)
	require.Same(t, advisor, AdvisorFromContext(ctx))
	require.Nil(t, AdvisorFromContext(context.TODO()))
}
