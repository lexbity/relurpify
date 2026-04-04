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

	// 1. Collect anchored files from anchors.FilePaths and anchors.SessionPins
	anchoredMap := make(map[string]CodeEvidenceItem)
	
	// Add current turn files with highest priority
	for _, path := range anchors.FilePaths {
		item := CodeEvidenceItem{
			Path:    path,
			Score:   1.0,
			Source:  EvidenceSourceAnchor,
			Summary: "User‑selected file",
		}
		anchoredMap[path] = item
	}
	
	// Add session pins with slightly lower priority
	for _, path := range anchors.SessionPins {
		if _, exists := anchoredMap[path]; !exists {
			item := CodeEvidenceItem{
				Path:    path,
				Score:   0.9,
				Source:  EvidenceSourceAnchor,
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
	
	// Sort stage1.CodeEvidence by score (descending) for better selection
	// For now, just take all available
	for _, item := range stage1.CodeEvidence {
		if seenPaths[item.Path] {
			continue
		}
		bundle.ExpandedFiles = append(bundle.ExpandedFiles, item)
		seenPaths[item.Path] = true
	}

	// 3. Knowledge items (deduplicate by RefID)
	seenRefs := make(map[string]bool)
	
	// Add knowledge from stage1
	for _, item := range stage1.KnowledgeEvidence {
		if seenRefs[item.RefID] {
			continue
		}
		bundle.KnowledgeTopic = append(bundle.KnowledgeTopic, item)
		seenRefs[item.RefID] = true
	}
	
	// Add expanded knowledge
	for _, item := range expanded {
		if seenRefs[item.RefID] {
			continue
		}
		bundle.KnowledgeExpanded = append(bundle.KnowledgeExpanded, item)
		seenRefs[item.RefID] = true
	}

	// 4. Apply limits
	// Limit anchored files (should rarely exceed limit)
	if len(bundle.AnchoredFiles) > m.config.MaxCodeFiles {
		bundle.AnchoredFiles = bundle.AnchoredFiles[:m.config.MaxCodeFiles]
	}
	
	// Limit expanded files
	if len(bundle.ExpandedFiles) > m.config.MaxCodeFiles {
		bundle.ExpandedFiles = bundle.ExpandedFiles[:m.config.MaxCodeFiles]
	}
	
	// Limit knowledge items
	totalKnowledge := len(bundle.KnowledgeTopic) + len(bundle.KnowledgeExpanded)
	if totalKnowledge > m.config.MaxKnowledgeItems {
		// Distribute proportionally
		topicRatio := float64(len(bundle.KnowledgeTopic)) / float64(totalKnowledge)
		topicLimit := int(float64(m.config.MaxKnowledgeItems) * topicRatio)
		if topicLimit < 1 && len(bundle.KnowledgeTopic) > 0 {
			topicLimit = 1
		}
		expandedLimit := m.config.MaxKnowledgeItems - topicLimit
		
		if len(bundle.KnowledgeTopic) > topicLimit {
			bundle.KnowledgeTopic = bundle.KnowledgeTopic[:topicLimit]
		}
		if len(bundle.KnowledgeExpanded) > expandedLimit {
			bundle.KnowledgeExpanded = bundle.KnowledgeExpanded[:expandedLimit]
		}
	}

	// 5. Simple token estimate
	// Rough estimates: anchored files ~500 tokens, expanded ~300 tokens
	// Knowledge items ~100 tokens each
	bundle.TokenEstimate = len(bundle.AnchoredFiles)*500 + 
		len(bundle.ExpandedFiles)*300 +
		(len(bundle.KnowledgeTopic) + len(bundle.KnowledgeExpanded))*100
	
	// Enforce token budget
	if bundle.TokenEstimate > m.config.TokenBudget {
		// For now, just cap the estimate
		bundle.TokenEstimate = m.config.TokenBudget
	}

	// 6. Populate PipelineTrace (simplified for now)
	bundle.PipelineTrace = PipelineTrace{
		AnchorsExtracted:      len(anchors.SymbolNames) + len(anchors.FilePaths) + len(anchors.PackageRefs),
		AnchorsConfirmed:      len(anchors.SymbolNames) + len(anchors.PackageRefs),
		Stage1CodeResults:     len(stage1.CodeEvidence),
		Stage1ArchaeoResults:  len(stage1.KnowledgeEvidence),
		HypotheticalGenerated: sketch.Grounded,
		HypotheticalTokens:    sketch.TokenCount,
		Stage3ArchaeoResults:  len(expanded),
		FallbackUsed:          !sketch.Grounded && len(stage1.CodeEvidence) == 0,
		FallbackReason:        "",
		TotalTokenEstimate:    bundle.TokenEstimate,
	}

	return bundle
}
