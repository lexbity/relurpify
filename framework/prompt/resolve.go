package prompt

import (
	"sort"
	"strings"
)

// assemble runs the full assembly pipeline for cfg against ctx using providers.
//
// Order of operations:
//  1. Merge inheritance
//  2. Inject provider content for SourceProvider blocks
//  3. Evaluate when-expressions; drop false blocks
//  4. Sort ascending by order (ties by slice index / file position)
//  5. Interpolate variables in static block content
//  6. Join non-empty parts with "\n\n"
func assemble(
	cfg *PromptConfig,
	ctx RuntimeContext,
	providers map[string]ContextProvider,
) (result string, included []BlockTrace, excluded []BlockTrace, err error) {

	blocks, _, resolveErr := mergeInheritance(cfg)
	if resolveErr != nil {
		return "", nil, nil, resolveErr
	}

	vars := mergeVariables(cfg)
	defaults := make(map[string]string, len(vars))
	for k, v := range vars {
		defaults[k] = v.Default
	}
	rtVars := ctx.Variables
	if rtVars == nil {
		rtVars = make(map[string]string)
	}
	state := ctx.State
	if state == nil {
		state = make(map[string]any)
	}

	type assembled struct {
		part  string
		block PromptBlock
		idx   int
	}
	var parts []assembled

	for i, b := range blocks {
		// Evaluate when-expression.
		if b.When != nil {
			include, evalErr := b.When.Evaluate(state)
			if evalErr != nil {
				excluded = append(excluded, BlockTrace{
					BlockID: b.ID,
					Source:  b.From,
					Order:   b.Order,
					Reason:  "when-expression error: " + evalErr.Error(),
				})
				continue
			}
			if !include {
				excluded = append(excluded, BlockTrace{
					BlockID: b.ID,
					Source:  b.From,
					Order:   b.Order,
					Reason:  "when-expression false",
				})
				continue
			}
		}

		var part string
		if b.From == SourceProvider {
			p, ok := providers[b.Provider]
			if !ok {
				excluded = append(excluded, BlockTrace{
					BlockID: b.ID,
					Source:  b.From,
					Order:   b.Order,
					Reason:  "provider not registered: " + b.Provider,
				})
				continue
			}
			var chunk ContextChunk
			if fp, isFailable := p.(FailableProvider); isFailable {
				var ferr error
				chunk, ferr = fp.ProvideOrFail(ctx)
				if ferr != nil {
					excluded = append(excluded, BlockTrace{
						BlockID: b.ID,
						Source:  b.From,
						Order:   b.Order,
						Reason:  "provider error: " + ferr.Error(),
					})
					continue
				}
			} else {
				chunk = p.Provide(ctx)
			}
			if chunk.Content == "" {
				excluded = append(excluded, BlockTrace{
					BlockID: b.ID,
					Source:  b.From,
					Order:   b.Order,
					Reason:  "provider returned empty content",
				})
				continue
			}
			part = chunk.Content
		} else {
			part = interpolate(b.Content, rtVars, defaults)
		}

		parts = append(parts, assembled{part: part, block: b, idx: i})
		included = append(included, BlockTrace{
			BlockID: b.ID,
			Source:  b.From,
			Order:   b.Order,
			Reason:  "included",
		})
	}

	// Sort by order, ties by original slice index.
	sort.SliceStable(parts, func(i, j int) bool {
		if parts[i].block.Order != parts[j].block.Order {
			return parts[i].block.Order < parts[j].block.Order
		}
		return parts[i].idx < parts[j].idx
	})

	var out []string
	for _, a := range parts {
		if strings.TrimSpace(a.part) != "" {
			out = append(out, a.part)
		}
	}

	return strings.Join(out, "\n\n"), included, excluded, nil
}
