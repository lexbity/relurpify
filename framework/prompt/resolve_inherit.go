package prompt

import "fmt"

const maxInheritanceDepth = 8

// mergeInheritance returns the effective block list for cfg by walking the
// ParentResolved chain. Must be called after all parents have been resolved.
//
// Merge rules (applied from root ancestor to cfg):
//   - Child blocks with the same id override the parent block.
//   - Locked parent blocks cannot be overridden — error recorded.
//   - Child blocks with new ids append to the set.
func mergeInheritance(cfg *PromptConfig) ([]PromptBlock, []ValidationIssue, error) {
	chain, err := buildInheritanceChain(cfg)
	if err != nil {
		return nil, nil, err
	}
	if len(chain) == 1 {
		return cfg.Blocks, nil, nil
	}

	var issues []ValidationIssue
	var effective []PromptBlock
	for _, c := range chain {
		effective, issues = applyChildBlocks(effective, c.Blocks, c.ID, issues)
	}
	return effective, issues, nil
}

// buildInheritanceChain returns [root, ..., cfg] — root ancestor first.
func buildInheritanceChain(cfg *PromptConfig) ([]*PromptConfig, error) {
	var chain []*PromptConfig
	seen := make(map[string]bool)
	cur := cfg
	for cur != nil {
		if seen[cur.ID] {
			return nil, fmt.Errorf("circular inheritance detected for prompt %s", cur.ID)
		}
		seen[cur.ID] = true
		chain = append([]*PromptConfig{cur}, chain...)
		cur = cur.ParentResolved
		if len(chain) > maxInheritanceDepth {
			return nil, fmt.Errorf("inheritance depth exceeds limit of %d for prompt %s",
				maxInheritanceDepth, cfg.ID)
		}
	}
	return chain, nil
}

// applyChildBlocks merges childBlocks onto parentBlocks.
func applyChildBlocks(parent, child []PromptBlock, childID string, issues []ValidationIssue) ([]PromptBlock, []ValidationIssue) {
	parentIdx := make(map[string]int, len(parent))
	for i, b := range parent {
		parentIdx[b.ID] = i
	}

	result := make([]PromptBlock, len(parent))
	copy(result, parent)

	for _, cb := range child {
		if pi, ok := parentIdx[cb.ID]; ok {
			if result[pi].Locked {
				issues = append(issues, ValidationIssue{
					PromptID: childID,
					BlockID:  cb.ID,
					Severity: SeverityError,
					Message:  "cannot override locked parent block: " + cb.ID,
				})
				continue
			}
			result[pi] = cb
		} else {
			result = append(result, cb)
		}
	}
	return result, issues
}

// mergeVariables returns the effective variable map by walking the chain.
// Child declarations win on collision.
func mergeVariables(cfg *PromptConfig) map[string]VariableDecl {
	chain, err := buildInheritanceChain(cfg)
	if err != nil || len(chain) <= 1 {
		return cfg.Variables
	}
	merged := make(map[string]VariableDecl)
	// Apply root → cfg; later entries win.
	for _, c := range chain {
		for k, v := range c.Variables {
			merged[k] = v
		}
	}
	return merged
}
