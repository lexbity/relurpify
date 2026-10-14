package chainer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/model"
)

func TestLinkConstructors(t *testing.T) {
	parse := func(string) (any, error) { return nil, nil }

	link := NewLink("l1", "prompt text", []string{"a", "b"}, "out", parse)
	assert.Equal(t, "l1", link.Name)
	assert.Equal(t, "prompt text", link.SystemPrompt)
	assert.Equal(t, []string{"a", "b"}, link.InputKeys)
	assert.Equal(t, "out", link.OutputKey)
	assert.NotNil(t, link.Parse)
	assert.Equal(t, FailurePolicyRetry, link.OnFailure)
	assert.Equal(t, 1, link.MaxRetries)

	// Input keys must be copied, not aliased.
	inputs := []string{"k"}
	link = NewLink("l2", "p", inputs, "out", nil)
	inputs[0] = "mutated"
	assert.Equal(t, []string{"k"}, link.InputKeys)

	summarize := NewSummarizeLink("sum", nil, "sum.out")
	assert.Equal(t, "Summarize the available input.", summarize.SystemPrompt)
	assert.Nil(t, summarize.Parse)

	transform := NewTransformLink("t", nil, "t.out", parse)
	assert.Equal(t, "Transform the available input.", transform.SystemPrompt)
	assert.NotNil(t, transform.Parse)
}

func TestChainValidate(t *testing.T) {
	tests := []struct {
		name    string
		chain   *Chain
		wantErr string
	}{
		{name: "nil chain rejected", chain: nil, wantErr: "chainer: chain required"},
		{name: "empty chain valid", chain: &Chain{}},
		{
			name:    "missing link name rejected",
			chain:   &Chain{Links: []Link{{OutputKey: "out"}}},
			wantErr: "chainer: link name required",
		},
		{
			name:    "missing output key rejected",
			chain:   &Chain{Links: []Link{{Name: "l"}}},
			wantErr: "chainer: link output key required",
		},
		{
			name: "output key in input keys rejected",
			chain: &Chain{Links: []Link{{
				Name:      "l",
				OutputKey: "out",
				InputKeys: []string{"out"},
			}}},
			wantErr: "chainer: link l output key cannot reference itself",
		},
		{
			name: "valid chain accepted",
			chain: &Chain{Links: []Link{{
				Name:      "l",
				OutputKey: "out",
				InputKeys: []string{"in"},
			}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.chain.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

func chainRunnerFor(m model.LanguageModel) *chainRunner {
	return &chainRunner{Model: m}
}

func TestChainRunnerRunRequiresModelAndValidChain(t *testing.T) {
	ctx := context.Background()

	require.ErrorContains(t, (*chainRunner)(nil).Run(ctx, nil, &Chain{}, nil), "model unavailable")
	require.ErrorContains(t, chainRunnerFor(nil).Run(ctx, nil, &Chain{}, nil), "model unavailable")

	runner := chainRunnerFor(newScriptChatModel("x"))
	err := runner.Run(ctx, nil, &Chain{Links: []Link{{OutputKey: "out"}}}, newTestEnvelope(t))
	require.ErrorContains(t, err, "link name required")

	// An empty chain is valid and runs zero links.
	require.NoError(t, runner.Run(ctx, nil, &Chain{}, newTestEnvelope(t)))
}

func TestChainRunnerRunExecutesLinksInOrder(t *testing.T) {
	env := newTestEnvelope(t)
	env.SetWorkingValueWithClass("research.finding", "the finding", contextdata.MemoryClassTask)

	first := NewLink("first", "First: {{.Instruction}}", []string{"research.finding"}, "first.out", nil)
	second := NewSummarizeLink("second", nil, "second.out")
	mdl := newScriptChatModel("first result", "second result")
	runner := chainRunnerFor(mdl)

	task := taskWith("do the work")
	require.NoError(t, runner.Run(context.Background(), task, &Chain{Links: []Link{first, second}}, env))

	// Both outputs land on the envelope under their declared keys.
	assert.Equal(t, "first result", envValue[string](t, env, "first.out"))
	assert.Equal(t, "second result", envValue[string](t, env, "second.out"))

	// Each link sent a system+user message pair, in declared order.
	require.Len(t, mdl.calls, 2)
	sys1 := mdl.calls[0][0]
	assert.Equal(t, "system", sys1.Role)
	assert.Contains(t, sys1.Content, "First: do the work", "template renders the task instruction")
	assert.Equal(t, "user", mdl.calls[0][1].Role)
	assert.Equal(t, "do the work", mdl.calls[0][1].Content)

	sys2 := mdl.calls[1][0]
	assert.Equal(t, "Summarize the available input.", sys2.Content)

	// Interactions record each assistant response with its link name.
	interactions := envInteractions(t, env)
	require.Len(t, interactions, 2)
	assert.Equal(t, "first", interactions[0]["link"])
	assert.Equal(t, "first result", interactions[0]["content"])
	assert.Equal(t, "second", interactions[1]["link"])
}

func TestChainRunnerRunRetriesParseFailureThenSucceeds(t *testing.T) {
	attempt := 0
	link := NewLink("strict", "p", nil, "out", func(text string) (any, error) {
		attempt++
		if attempt == 1 {
			return nil, errors.New("bad shape")
		}
		return "parsed: " + text, nil
	})
	link.MaxRetries = 1
	mdl := newScriptChatModel("first try", "second try")
	env := newTestEnvelope(t)

	require.NoError(t, chainRunnerFor(mdl).Run(context.Background(), taskWith("goal"), &Chain{Links: []Link{link}}, env))

	assert.Equal(t, 2, mdl.chatCalls(), "one retry after the parse failure")
	assert.Equal(t, 2, attempt)

	last := mdl.lastMessages()
	require.Len(t, last, 2)
	assert.Contains(t, last[1].Content, "Previous response could not be parsed: bad shape")
	assert.Contains(t, last[1].Content, "Return a corrected response")

	assert.Equal(t, "parsed: second try", envValue[string](t, env, "out"))
}

func TestChainRunnerRunFailFastPolicyReturnsImmediately(t *testing.T) {
	link := NewLink("strict", "p", nil, "out", func(string) (any, error) {
		return nil, errors.New("nope")
	})
	link.OnFailure = FailurePolicyFailFast
	mdl := newScriptChatModel("only try")

	err := chainRunnerFor(mdl).Run(context.Background(), nil, &Chain{Links: []Link{link}}, newTestEnvelope(t))
	require.ErrorIs(t, err, ErrLinkParseFailure)
	assert.Contains(t, err.Error(), "nope")
	assert.Equal(t, 1, mdl.chatCalls(), "fail_fast must not retry")
}

func TestChainRunnerRunRetryExhaustionReturnsParseFailure(t *testing.T) {
	link := NewLink("strict", "p", nil, "out", func(string) (any, error) {
		return nil, errors.New("always bad")
	})
	link.MaxRetries = 1
	mdl := newScriptChatModel("a", "b")

	err := chainRunnerFor(mdl).Run(context.Background(), nil, &Chain{Links: []Link{link}}, newTestEnvelope(t))
	require.ErrorIs(t, err, ErrLinkParseFailure)
	assert.Equal(t, 2, mdl.chatCalls(), "initial attempt plus one retry")
}

func TestChainRunnerRunChatErrorPropagates(t *testing.T) {
	link := NewSummarizeLink("l", nil, "out")
	mdl := newScriptChatModel()
	mdl.responses = []chatResponse{{err: errors.New("backend down")}}

	err := chainRunnerFor(mdl).Run(context.Background(), nil, &Chain{Links: []Link{link}}, newTestEnvelope(t))
	require.ErrorContains(t, err, "chainer: link l")
	require.ErrorContains(t, err, "backend down")
}

func TestRunChainExportedEntry(t *testing.T) {
	env := newTestEnvelope(t)
	chain := &Chain{Links: []Link{NewSummarizeLink("l", nil, "out")}}
	mdl := newScriptChatModel("ran via RunChain")

	require.NoError(t, RunChain(context.Background(), mdl, nil, chain, env, nil))
	assert.Equal(t, "ran via RunChain", envValue[string](t, env, "out"))
}

func TestResolveSystemPromptPrefersRegistryPromptID(t *testing.T) {
	reg := &resolvingRegistry{resolved: map[string]string{"chainer.special": "registry prompt body"}}
	link := Link{
		Name:         "l",
		SystemPrompt: "inline fallback",
		PromptID:     "chainer.special",
		InputKeys:    []string{"state.key"},
	}

	resolved, err := resolveSystemPrompt(link, taskWith("goal"), newTestEnvelope(t), reg)
	require.NoError(t, err)
	assert.Equal(t, "registry prompt body", resolved)
	assert.Equal(t, []string{"chainer.special"}, reg.seenIDs)
}

func TestResolveSystemPromptFallsBackToInlineOnRegistryError(t *testing.T) {
	reg := &resolvingRegistry{err: errors.New("lookup failed")}
	link := Link{Name: "l", SystemPrompt: "inline fallback", PromptID: "missing.id"}

	resolved, err := resolveSystemPrompt(link, taskWith("goal"), newTestEnvelope(t), reg)
	require.NoError(t, err)
	assert.Equal(t, "inline fallback", resolved)
}

func TestResolveSystemPromptInlineWhenNoPromptID(t *testing.T) {
	link := Link{
		Name:         "l",
		SystemPrompt: "Instruction={{.Instruction}} Input={{ index .Input \"k\" }}",
		InputKeys:    []string{"k"},
	}
	env := newTestEnvelope(t)
	env.SetWorkingValueWithClass("k", "v", contextdata.MemoryClassTask)

	resolved, err := resolveSystemPrompt(link, taskWith("goal"), env, nil)
	require.NoError(t, err)
	assert.Contains(t, resolved, "Instruction=goal")
	assert.Contains(t, resolved, "Input=v", "declared InputKeys feed the rendered prompt")
}

func TestResolveSystemPromptRendersLinkInputKeysIntoPromptInput(t *testing.T) {
	link := Link{
		Name:         "l",
		SystemPrompt: "keys: {{ range $k, $v := .Input }}{{ $k }}={{ $v }} {{ end }}",
		InputKeys:    []string{"a", "b"},
	}
	env := newTestEnvelope(t)
	env.SetWorkingValueWithClass("a", "1", contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("b", "2", contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("c", "filtered-out", contextdata.MemoryClassTask)

	resolved, err := resolveSystemPrompt(link, nil, env, nil)
	require.NoError(t, err)
	assert.Contains(t, resolved, "a=1")
	assert.Contains(t, resolved, "b=2")
	assert.NotContains(t, resolved, "filtered-out")
}

func TestRenderLinkPromptErrors(t *testing.T) {
	_, err := renderLinkPrompt("{{ .Unclosed", "goal", nil)
	require.Error(t, err, "template parse errors must surface")

	_, err = renderLinkPrompt("{{ .Bogus }}", "goal", nil)
	require.Error(t, err, "template execution errors must surface")
}

func TestTaskInstruction(t *testing.T) {
	assert.Equal(t, "", taskInstruction(nil))
	assert.Equal(t, "instructions", taskInstruction(taskWith("instructions")))
}

func TestParseLinkResponse(t *testing.T) {
	out, err := parseLinkResponse(Link{}, "raw text")
	require.NoError(t, err)
	assert.Equal(t, "raw text", out, "links without a parser pass text through")

	out, err = parseLinkResponse(Link{Parse: func(string) (any, error) {
		return 42, nil
	}}, "ignored")
	require.NoError(t, err)
	assert.Equal(t, 42, out)

	_, err = parseLinkResponse(Link{Parse: func(string) (any, error) {
		return nil, errors.New("bad")
	}}, "x")
	require.Error(t, err)
}

func TestLinkFailurePolicyDefaultsToRetry(t *testing.T) {
	assert.Equal(t, FailurePolicyRetry, linkFailurePolicy(Link{}))
	assert.Equal(t, FailurePolicyFailFast, linkFailurePolicy(Link{OnFailure: FailurePolicyFailFast}))
}

func envInteractions(t *testing.T, env *contextdata.Envelope) []map[string]any {
	t.Helper()
	raw, ok := env.WorkingDataSnapshot()["_interactions"]
	require.True(t, ok, "expected interactions on the envelope")
	interactions, ok := raw.([]map[string]any)
	require.True(t, ok, "unexpected interactions type %T", raw)
	return interactions
}

func envValue[T any](t *testing.T, env *contextdata.Envelope, key string) T {
	t.Helper()
	v, ok := contextdata.GetTyped[any](env, key)
	require.True(t, ok, "expected key %s on the envelope", key)
	typed, ok := v.(T)
	require.True(t, ok, "value under %s has unexpected type %T", key, v)
	return typed
}
