package relurpic

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

type commenterAnnotateCapabilityHandler struct {
	commentStore patterns.CommentStore
	patternStore patterns.PatternStore
	retrievalDB  *sql.DB
}

func (h commenterAnnotateCapabilityHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return coordinatedRelurpicDescriptor(
		"relurpic:commenter.annotate",
		"commenter.annotate",
		"Persist intent-typed annotations on patterns, anchors, files, or symbols and optionally promote them to anchors.",
		core.CapabilityKindTool,
		core.CoordinationRoleDomainPack,
		[]string{"annotate", "comment"},
		[]core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
		structuredObjectSchema(map[string]*core.Schema{
			"pattern_id":   {Type: "string"},
			"anchor_id":    {Type: "string"},
			"file_path":    {Type: "string"},
			"symbol_id":    {Type: "string"},
			"intent_type":  {Type: "string"},
			"body":         {Type: "string"},
			"author_kind":  {Type: "string"},
			"corpus_scope": {Type: "string"},
		}, "intent_type", "body", "author_kind"),
		structuredObjectSchema(map[string]*core.Schema{
			"comment_id": {Type: "string"},
			"anchor_ref": {Type: "string"},
		}, "comment_id"),
		map[string]any{
			"relurpic_capability": true,
			"workflow":            "comment",
		},
		[]core.RiskClass{core.RiskClassReadOnly},
		[]core.EffectClass{core.EffectClassContextInsertion, core.EffectClassFilesystemMutation},
	)
}

func (h commenterAnnotateCapabilityHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]any) (*core.CapabilityExecutionResult, error) {
	if h.commentStore == nil {
		return nil, fmt.Errorf("comment store unavailable")
	}
	patternID := stringArg(args["pattern_id"])
	anchorID := stringArg(args["anchor_id"])
	filePath := stringArg(args["file_path"])
	symbolID := stringArg(args["symbol_id"])
	if patternID == "" && anchorID == "" && filePath == "" && symbolID == "" {
		return nil, fmt.Errorf("at least one of pattern_id, anchor_id, file_path, or symbol_id is required")
	}

	intentType, err := normalizeCommentIntentType(args["intent_type"])
	if err != nil {
		return nil, err
	}
	authorKind, err := normalizeAuthorKind(args["author_kind"])
	if err != nil {
		return nil, err
	}
	body := stringArg(args["body"])
	if body == "" {
		return nil, fmt.Errorf("body required")
	}
	corpusScope := stringArg(args["corpus_scope"])

	now := time.Now().UTC()
	record := patterns.CommentRecord{
		CommentID:   commentID(patternID, anchorID, filePath, symbolID, string(intentType), body),
		PatternID:   patternID,
		AnchorID:    anchorID,
		FilePath:    filePath,
		SymbolID:    symbolID,
		IntentType:  intentType,
		Body:        body,
		AuthorKind:  authorKind,
		TrustClass:  trustClassForAuthor(authorKind),
		CorpusScope: corpusScope,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if h.retrievalDB != nil {
		if anchorRef, err := h.promoteCommentAnchor(ctx, record); err != nil {
			return nil, err
		} else if anchorRef != "" {
			record.AnchorRef = anchorRef
		}
	}

	if err := h.commentStore.Save(ctx, record); err != nil {
		return nil, err
	}
	if err := h.appendCommentToPattern(ctx, record); err != nil {
		return nil, err
	}

	data := map[string]any{
		"comment_id": record.CommentID,
	}
	if record.AnchorRef != "" {
		data["anchor_ref"] = record.AnchorRef
	}
	return &core.CapabilityExecutionResult{Success: true, Data: data}, nil
}

func normalizeCommentIntentType(raw any) (patterns.CommentIntentType, error) {
	switch stringArg(raw) {
	case string(patterns.CommentIntentional):
		return patterns.CommentIntentional, nil
	case string(patterns.CommentDeferred):
		return patterns.CommentDeferred, nil
	case string(patterns.CommentOpenQuestion):
		return patterns.CommentOpenQuestion, nil
	case string(patterns.CommentSuperseding):
		return patterns.CommentSuperseding, nil
	case string(patterns.CommentBoundaryConstraint):
		return patterns.CommentBoundaryConstraint, nil
	default:
		return "", fmt.Errorf("unsupported intent_type")
	}
}

func normalizeAuthorKind(raw any) (patterns.AuthorKind, error) {
	switch stringArg(raw) {
	case string(patterns.AuthorKindHuman):
		return patterns.AuthorKindHuman, nil
	case string(patterns.AuthorKindAgent):
		return patterns.AuthorKindAgent, nil
	default:
		return "", fmt.Errorf("unsupported author_kind")
	}
}

func trustClassForAuthor(kind patterns.AuthorKind) patterns.TrustClass {
	if kind == patterns.AuthorKindHuman {
		return patterns.TrustClassWorkspaceTrusted
	}
	return patterns.TrustClassBuiltinTrusted
}

func (h commenterAnnotateCapabilityHandler) promoteCommentAnchor(ctx context.Context, record patterns.CommentRecord) (string, error) {
	if !shouldPromoteComment(record.IntentType, record.Body) {
		return "", nil
	}
	term, definition, ok := splitCommentAnchorBody(record.Body)
	if !ok {
		return "", nil
	}
	anchorClass := "commitment"
	if record.IntentType == patterns.CommentBoundaryConstraint {
		anchorClass = "policy"
	}
	anchor, err := retrieval.DeclareAnchor(ctx, h.retrievalDB, retrieval.AnchorDeclaration{
		Term:       term,
		Definition: definition,
		Class:      anchorClass,
		Context: map[string]string{
			"comment_id": record.CommentID,
			"file_path":  record.FilePath,
			"symbol_id":  record.SymbolID,
		},
	}, record.CorpusScope, string(patterns.TrustClassWorkspaceTrusted))
	if err != nil {
		return "", err
	}
	return anchor.AnchorID, nil
}

func shouldPromoteComment(intentType patterns.CommentIntentType, body string) bool {
	if intentType != patterns.CommentIntentional && intentType != patterns.CommentBoundaryConstraint {
		return false
	}
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 200 {
		return false
	}
	return strings.Contains(body, ":") || strings.Contains(body, " - ") || strings.Contains(body, " -- ")
}

func splitCommentAnchorBody(body string) (string, string, bool) {
	body = strings.TrimSpace(body)
	for _, sep := range []string{":", " -- ", " - "} {
		if !strings.Contains(body, sep) {
			continue
		}
		parts := strings.SplitN(body, sep, 2)
		term := strings.TrimSpace(parts[0])
		definition := strings.TrimSpace(parts[1])
		if term == "" || definition == "" {
			return "", "", false
		}
		return term, definition, true
	}
	return "", "", false
}

func (h commenterAnnotateCapabilityHandler) appendCommentToPattern(ctx context.Context, record patterns.CommentRecord) error {
	if h.patternStore == nil || record.PatternID == "" {
		return nil
	}
	pattern, err := h.patternStore.Load(ctx, record.PatternID)
	if err != nil {
		return err
	}
	if pattern == nil {
		return nil
	}
	pattern.CommentIDs = appendIfMissing(pattern.CommentIDs, record.CommentID)
	if record.AnchorRef != "" {
		pattern.AnchorRefs = appendIfMissing(pattern.AnchorRefs, record.AnchorRef)
	}
	pattern.UpdatedAt = time.Now().UTC()
	return h.patternStore.Save(ctx, *pattern)
}

func commentID(patternID, anchorID, filePath, symbolID, intentType, body string) string {
	sum := sha1.Sum([]byte(strings.Join([]string{patternID, anchorID, filePath, symbolID, intentType, body}, "|")))
	return fmt.Sprintf("comment-%x", sum[:8])
}
