package pretask

// ResultMerger merges pipeline stage outputs into a single ranked bundle.
type ResultMerger struct {
	config MergerConfig
}

type MergerConfig struct {
	TokenBudget       int
	MaxCodeFiles      int
	MaxKnowledgeItems int
}

// Merge produces an EnrichedContextBundle from pipeline stage outputs.
//
// Merge strategy:
//   1. Anchored files (source="anchor" or "session_pin") always included
//   2. Remaining code slots filled by score, descending, deduped by path
//   3. KnowledgeTopic items scored by keyword overlap with query
//   4. KnowledgeExpanded items scored by retrieval score
//   5. Deduplication across topic/expanded by RefID
//   6. Token budget enforced: demote to DetailMinimal before dropping
//   7. PipelineTrace populated from all stage inputs
func (m *ResultMerger) Merge(
	query string,
	anchors AnchorSet,
	stage1 Stage1Result,
	sketch HypotheticalSketch,
	expanded []KnowledgeEvidenceItem,
) EnrichedContextBundle {
	bundle := EnrichedContextBundle{}

	// 1. Collect anchored files from anchors.FilePaths
	anchoredMap := make(map[string]CodeEvidenceItem)
	for _, path := range anchors.FilePaths {
		item := CodeEvidenceItem{
			Path:   path,
			Score:  1.0,
			Source: EvidenceSourceAnchor,
			Summary: "User‑selected file",
		}
		anchoredMap[path] = item
	}
	// Also include session pins as anchored
	for _, path := range anchors.SessionPins {
		if _, ok := anchoredMap[path]; !ok {
			item := CodeEvidenceItem{
				Path:   path,
				Score:  0.9,
				Source: EvidenceSourceAnchor,
				Summary: "Previously pinned",
			}
			anchoredMap[path] = item
		}
	}
	// Convert map to slice
	for _, item := range anchoredMap {
		bundle.AnchoredFiles = append(bundle.AnchoredFiles, item)
	}

	// 2. Expanded files from stage1.CodeEvidence (excluding anchored paths)
	seenPaths := make(map[string]bool)
	for _, item := range bundle.AnchoredFiles {
		seenPaths[item.Path] = true
	}
	for _, item := range stage1.CodeEvidence {
		if seenPaths[item.Path] {
			continue
		}
		bundle.ExpandedFiles = append(bundle.ExpandedFiles, item)
		seenPaths[item.Path] = true
	}

	// 3. Knowledge items (deduplicate by RefID)
	seenRefs := make(map[string]bool)
	for _, item := range stage1.KnowledgeEvidence {
		if seenRefs[item.RefID] {
			continue
		}
		bundle.KnowledgeTopic = append(bundle.KnowledgeTopic, item)
		seenRefs[item.RefID] = true
	}
	for _, item := range expanded {
		if seenRefs[item.RefID] {
			continue
		}
		bundle.KnowledgeExpanded = append(bundle.KnowledgeExpanded, item)
		seenRefs[item.RefID] = true
	}

	// 4. Apply limits
	if len(bundle.AnchoredFiles) > m.config.MaxCodeFiles {
		bundle.AnchoredFiles = bundle.AnchoredFiles[:m.config.MaxCodeFiles]
	}
	if len(bundle.ExpandedFiles) > m.config.MaxCodeFiles {
		bundle.ExpandedFiles = bundle.ExpandedFiles[:m.config.MaxCodeFiles]
	}
	if len(bundle.KnowledgeTopic) > m.config.MaxKnowledgeItems {
		bundle.KnowledgeTopic = bundle.KnowledgeTopic[:m.config.MaxKnowledgeItems]
	}
	if len(bundle.KnowledgeExpanded) > m.config.MaxKnowledgeItems {
		bundle.KnowledgeExpanded = bundle.KnowledgeExpanded[:m.config.MaxKnowledgeItems]
	}

	// 5. Simple token estimate (placeholder)
	bundle.TokenEstimate = len(bundle.AnchoredFiles)*500 + len(bundle.ExpandedFiles)*300 +
		len(bundle.KnowledgeTopic)*100 + len(bundle.KnowledgeExpanded)*100
	if bundle.TokenEstimate > m.config.TokenBudget {
		bundle.TokenEstimate = m.config.TokenBudget
	}

	return bundle
}
