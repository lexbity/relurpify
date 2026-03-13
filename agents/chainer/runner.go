package chainer

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/lexcodex/relurpify/framework/core"
)

type chainRunner struct {
	Model   core.LanguageModel
	Options core.LLMOptions
}

// FilterState returns only the requested state keys.
func FilterState(state *core.Context, keys []string) map[string]any {
	filtered := make(map[string]any, len(keys))
	if state == nil {
		return filtered
	}
	for _, key := range keys {
		if value, ok := state.Get(key); ok {
			filtered[key] = value
		}
	}
	return filtered
}

// RunChain executes a chain against state using isolated prompts.
func RunChain(ctx context.Context, model core.LanguageModel, task *core.Task, chain *Chain, state *core.Context) error {
	return (&chainRunner{Model: model}).Run(ctx, task, chain, state)
}

func (r *chainRunner) Run(ctx context.Context, task *core.Task, chain *Chain, state *core.Context) error {
	if r == nil || r.Model == nil {
		return fmt.Errorf("chainer: model unavailable")
	}
	if state == nil {
		state = core.NewContext()
	}
	if err := chain.Validate(); err != nil {
		return err
	}
	for _, link := range chain.Links {
		filtered := FilterState(state, link.InputKeys)
		systemPrompt, err := renderLinkPrompt(link.SystemPrompt, taskInstruction(task), filtered)
		if err != nil {
			return fmt.Errorf("chainer: render link %s: %w", link.Name, err)
		}
		retries := 0
		maxRetries := link.MaxRetries
		if maxRetries <= 0 {
			maxRetries = 1
		}
		userPrompt := taskInstruction(task)
		for {
			resp, err := r.Model.Chat(ctx, []core.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: userPrompt},
			}, &r.Options)
			if err != nil {
				return fmt.Errorf("chainer: link %s: %w", link.Name, err)
			}
			parsed, parseErr := parseLinkResponse(link, resp.Text)
			if parseErr == nil {
				state.Set(link.OutputKey, parsed)
				state.AddInteraction("assistant", resp.Text, map[string]any{"link": link.Name})
				break
			}
			if linkFailurePolicy(link) == FailurePolicyRetry && retries < maxRetries {
				retries++
				userPrompt = taskInstruction(task) + "\nPrevious response could not be parsed: " + parseErr.Error() + "\nReturn a corrected response."
				continue
			}
			return fmt.Errorf("%w: %s", ErrLinkParseFailure, parseErr.Error())
		}
	}
	return nil
}

func parseLinkResponse(link Link, text string) (any, error) {
	if link.Parse == nil {
		return text, nil
	}
	return link.Parse(text)
}

func linkFailurePolicy(link Link) FailurePolicy {
	if link.OnFailure == "" {
		return FailurePolicyRetry
	}
	return link.OnFailure
}

func renderLinkPrompt(src, instruction string, input map[string]any) (string, error) {
	tpl, err := template.New("link").Parse(src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	data := struct {
		Instruction string
		Input       map[string]any
	}{
		Instruction: instruction,
		Input:       input,
	}
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func taskInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return task.Instruction
}
