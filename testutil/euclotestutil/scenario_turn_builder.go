package testutil

import "github.com/lexcodex/relurpify/framework/core"

type TurnBuilder struct {
	turn ScenarioModelTurn
}

func Turn(method string) *TurnBuilder {
	return &TurnBuilder{turn: ScenarioModelTurn{Method: method}}
}

func (b *TurnBuilder) Responding(text string) *TurnBuilder {
	if b.turn.Response == nil {
		b.turn.Response = &core.LLMResponse{}
	}
	b.turn.Response.Text = text
	return b
}

func (b *TurnBuilder) WithToolCall(name string, args map[string]interface{}) *TurnBuilder {
	if b.turn.Response == nil {
		b.turn.Response = &core.LLMResponse{}
	}
	b.turn.Response.ToolCalls = append(b.turn.Response.ToolCalls, core.ToolCall{
		Name: name,
		Args: cloneArgs(args),
	})
	return b
}

func (b *TurnBuilder) ExpectingPromptFragment(fragment string) *TurnBuilder {
	b.turn.PromptContains = append(b.turn.PromptContains, fragment)
	return b
}

func (b *TurnBuilder) ReturningError(err error) *TurnBuilder {
	b.turn.Err = err
	return b
}

func (b *TurnBuilder) Build() ScenarioModelTurn {
	turn := b.turn
	turn.PromptContains = append([]string(nil), b.turn.PromptContains...)
	if b.turn.Response != nil {
		resp := *b.turn.Response
		resp.ToolCalls = append([]core.ToolCall(nil), b.turn.Response.ToolCalls...)
		turn.Response = &resp
	}
	return turn
}

func cloneArgs(args map[string]interface{}) map[string]interface{} {
	if args == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(args))
	for k, v := range args {
		cloned[k] = v
	}
	return cloned
}
