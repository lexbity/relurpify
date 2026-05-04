package chainer

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type chainRunner struct {
	Model    contracts.LanguageModel
	Options  contracts.LLMOptions
	Registry interface{} // prompt.Registry - using interface{} to avoid import cycle
}

// FilterState returns only the requested state keys.
func FilterState(env *contextdata.Envelope, keys []string) map[string]any {
	if env == nil {
		return map[string]any{}
	}
	if len(keys) == 0 {
		return map[string]any{}
	}
	snapshot := env.HandoffSnapshot(contextdata.HandoffPolicy{
		PreserveWorkingMemory: true,
		WorkingKeys:           append([]string(nil), keys...),
	})
	if snapshot == nil {
		return map[string]any{}
	}
	return snapshot.WorkingData
}

// RunChain executes a chain against state using isolated prompts.
func RunChain(ctx context.Context, model contracts.LanguageModel, task *core.Task, chain *Chain, env *contextdata.Envelope, registry interface{}) error {
	runner := &chainRunner{
		Model:    model,
		Registry: registry,
	}
	return runner.Run(ctx, task, chain, env)
}

func (r *chainRunner) Run(ctx context.Context, task *core.Task, chain *Chain, env *contextdata.Envelope) error {
	if r == nil || r.Model == nil {
		return fmt.Errorf("chainer: model unavailable")
	}
	if env == nil {
		env = contextdata.NewEnvelope("chainer", "session")
	}
	if err := chain.Validate(); err != nil {
		return err
	}
	for _, link := range chain.Links {
		systemPrompt, err := resolveSystemPrompt(link, task, env, r.Registry)
		if err != nil {
			return fmt.Errorf("chainer: resolve link %s: %w", link.Name, err)
		}
		retries := 0
		maxRetries := link.MaxRetries
		if maxRetries <= 0 {
			maxRetries = 1
		}
		userPrompt := taskInstruction(task)
		for {
			resp, err := r.Model.Chat(ctx, []contracts.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: userPrompt},
			}, &r.Options)
			if err != nil {
				return fmt.Errorf("chainer: link %s: %w", link.Name, err)
			}
			parsed, parseErr := parseLinkResponse(link, resp.Text)
			if parseErr == nil {
				env.SetWorkingValue(link.OutputKey, parsed, contextdata.MemoryClassTask)
				env.AddInteraction(map[string]interface{}{
					"role":    "assistant",
					"content": resp.Text,
					"link":    link.Name,
				})
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

// resolveSystemPrompt returns the system prompt for a link, checking PromptID first.
func resolveSystemPrompt(link Link, task *core.Task, env *contextdata.Envelope, registry interface{}) (string, error) {
	// Check for registry-based resolution first
	if link.PromptID != "" && registry != nil {
		// Type assert to prompt.Registry interface
		if reg, ok := registry.(interface {
			Resolve(id string, ctx interface{}) (string, error)
		}); ok {
			// Build runtime context for chainer
			rctx := buildChainerRuntimeContext(task, env)
			if resolved, err := reg.Resolve(link.PromptID, rctx); err == nil {
				return resolved, nil
			}
			// Fall through to existing logic on error
		}
	}

	// Existing inline prompt path
	filtered := FilterState(env, link.InputKeys)
	return renderLinkPrompt(link.SystemPrompt, taskInstruction(task), filtered)
}

// buildChainerRuntimeContext creates a prompt.RuntimeContext for chainer links.
func buildChainerRuntimeContext(task *core.Task, env *contextdata.Envelope) interface{} {
	// Return a map that matches the expected RuntimeContext structure
	// Using interface{} to avoid import cycles with framework/prompt
	return map[string]interface{}{
		"Variables": map[string]string{
			"instruction": taskInstruction(task),
		},
		"State":        map[string]interface{}{},
		"Envelope":     env,
		"Paradigm":     "chainer",
		"ConsumerID":   "chainer",
		"Task":         task,
		"Tools":        []interface{}{}, // Tools not available at build time
		"Capabilities": []interface{}{}, // Capabilities not available at build time
		"AgentSpec":    nil,             // AgentSpec not available at build time
	}
}
