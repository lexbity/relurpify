package prompt

import (
	"fmt"
)

func resolvePrompt(cfg *PromptConfig, ctx RuntimeContext) (string, map[string]string, error) {
	if cfg == nil {
		return "", nil, fmt.Errorf("prompt config is nil")
	}

	resolved := make(map[string]string, len(cfg.Variables))
	for name, decl := range cfg.Variables {
		if ctx.Variables != nil {
			if value, ok := ctx.Variables[name]; ok {
				resolved[name] = value
				continue
			}
		}
		resolved[name] = decl.Default
	}

	out, err := renderMarkdownBody(cfg.Body, resolved)
	if err != nil {
		return "", nil, err
	}
	return out, resolved, nil
}
