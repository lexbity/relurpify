package blackboard

import (
	"encoding/json"
	"fmt"
	"strings"

	agentblackboard "codeburg.org/lexbit/relurpify/agents/blackboard"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

const bridgeSource = "euclo:blackboard_bridge"

var defaultArtifactMappings = map[euclotypes.ArtifactKind]string{
	euclotypes.ArtifactKindExplore:      "explore:workspace_state",
	euclotypes.ArtifactKindReproduction: "debug:reproduction",
	euclotypes.ArtifactKindRootCause:    "debug:root_cause",
	euclotypes.ArtifactKindVerification: "verify:result",
	euclotypes.ArtifactKindTrace:        "trace:raw_output",
}

type ArtifactBridge struct {
	board       *agentblackboard.Blackboard
	kindToEntry map[euclotypes.ArtifactKind]string
	entryToKind map[string]euclotypes.ArtifactKind
}

func NewArtifactBridge(board *agentblackboard.Blackboard) *ArtifactBridge {
	bridge := &ArtifactBridge{
		board:       board,
		kindToEntry: make(map[euclotypes.ArtifactKind]string, len(defaultArtifactMappings)),
		entryToKind: make(map[string]euclotypes.ArtifactKind, len(defaultArtifactMappings)),
	}
	for kind, entry := range defaultArtifactMappings {
		bridge.RegisterMapping(kind, entry)
	}
	return bridge
}

func (b *ArtifactBridge) RegisterMapping(kind euclotypes.ArtifactKind, entry string) {
	if b == nil {
		return
	}
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return
	}
	b.kindToEntry[kind] = entry
	b.entryToKind[entry] = kind
}

func (b *ArtifactBridge) SeedFromArtifacts(artifacts euclotypes.ArtifactState) error {
	if b == nil || b.board == nil {
		return fmt.Errorf("blackboard bridge requires a board")
	}
	for _, artifact := range artifacts.All() {
		entry, ok := b.kindToEntry[artifact.Kind]
		if !ok {
			continue
		}
		if !setBoardEntry(b.board, entry, artifact.Payload, bridgeSource) {
			return fmt.Errorf("seed blackboard entry %q", entry)
		}
	}
	return nil
}

func (b *ArtifactBridge) HarvestToArtifacts() []euclotypes.Artifact {
	if b == nil || b.board == nil {
		return nil
	}
	var artifacts []euclotypes.Artifact
	seen := map[string]struct{}{}
	for entry, kind := range b.entryToKind {
		if value, ok := boardEntryValue(b.board, entry); ok {
			id := strings.NewReplacer(":", "_", ".", "_").Replace(entry)
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         id,
				Kind:       kind,
				Summary:    summarizePayload(value),
				Payload:    value,
				ProducerID: bridgeSource,
				Status:     "produced",
				Metadata:   map[string]any{"blackboard_entry": entry},
			})
			seen[entry] = struct{}{}
		}
	}
	for _, artifact := range b.board.Artifacts {
		if _, ok := seen[artifact.Kind]; ok {
			continue
		}
		kind, ok := b.entryToKind[artifact.Kind]
		if !ok {
			continue
		}
		payload := decodeValue(artifact.Content)
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         firstNonEmpty(strings.TrimSpace(artifact.ID), strings.NewReplacer(":", "_", ".", "_").Replace(artifact.Kind)),
			Kind:       kind,
			Summary:    summarizePayload(payload),
			Payload:    payload,
			ProducerID: artifact.Source,
			Status:     "produced",
			Metadata:   map[string]any{"blackboard_entry": artifact.Kind, "verified": artifact.Verified},
		})
	}
	return artifacts
}

func setBoardEntry(board *agentblackboard.Blackboard, entry string, value any, source string) bool {
	if board == nil {
		return false
	}
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return false
	}
	encoded := encodeValue(value)
	for i := range board.Facts {
		if board.Facts[i].Key == entry {
			board.Facts[i].Value = encoded
			board.Facts[i].Source = source
			return true
		}
	}
	return board.AddFact(entry, encoded, source)
}

func boardEntryValue(board *agentblackboard.Blackboard, entry string) (any, bool) {
	if board == nil {
		return nil, false
	}
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil, false
	}
	for i := len(board.Facts) - 1; i >= 0; i-- {
		if board.Facts[i].Key == entry {
			return decodeValue(board.Facts[i].Value), true
		}
	}
	for i := len(board.Artifacts) - 1; i >= 0; i-- {
		if board.Artifacts[i].Kind == entry {
			return decodeValue(board.Artifacts[i].Content), true
		}
	}
	return nil, false
}

func boardHasEntry(board *agentblackboard.Blackboard, entry string) bool {
	_, ok := boardEntryValue(board, entry)
	return ok
}

func encodeValue(value any) string {
	data, err := json.Marshal(value)
	if err == nil {
		return string(data)
	}
	return fmt.Sprintf("%v", value)
}

func decodeValue(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		return payload
	}
	return raw
}

func summarizePayload(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		encoded := encodeValue(value)
		if len(encoded) > 160 {
			return encoded[:157] + "..."
		}
		return encoded
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
