package planner

import (
	"context"
	"errors"

	"github.com/lexcodex/relurpify/framework/core"
)

type stubLLM struct {
	responses []*core.LLMResponse
	idx       int
}

func boolPtr(value bool) *bool { return &value }

func (s *stubLLM) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return s.nextResponse()
}

func (s *stubLLM) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (s *stubLLM) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *stubLLM) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return s.nextResponse()
}

func (s *stubLLM) nextResponse() (*core.LLMResponse, error) {
	if s.idx >= len(s.responses) {
		return nil, errors.New("no response")
	}
	resp := s.responses[s.idx]
	s.idx++
	return resp, nil
}

type stubTool struct {
	name   string
	tags   []string
	params []core.ToolParameter
}

func (t stubTool) Name() string        { return t.name }
func (t stubTool) Description() string { return "stub tool" }
func (t stubTool) Category() string    { return "test" }

func (t stubTool) Parameters() []core.ToolParameter {
	if t.params != nil {
		return t.params
	}
	return []core.ToolParameter{{Name: "value", Type: "string", Required: false}}
}

func (t stubTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]interface{}{}}, nil
}

func (t stubTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t stubTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (t stubTool) Tags() []string                                  { return t.tags }
