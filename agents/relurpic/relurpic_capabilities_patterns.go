package relurpic

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/patterns"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

type patternDetectorDetectCapabilityHandler struct {
	model        core.LanguageModel
	config       *core.Config
	registry     *capability.Registry
	indexManager *ast.IndexManager
	graphDB      *graphdb.Engine
	patternStore patterns.PatternStore
	retrievalDB  *sql.DB
}

func (h patternDetectorDetectCapabilityHandler) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return coordinatedRelurpicDescriptor(
		"relurpic:pattern-detector.detect",
		"pattern-detector.detect",
		"Inspect a file or symbol scope and return grounded pattern proposals.",
		core.CapabilityKindTool,
		core.CoordinationRoleDomainPack,
		[]string{"analyze", "pattern-detect"},
		[]core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
		structuredObjectSchema(map[string]*core.Schema{
			"symbol_scope":  {Type: "string"},
			"corpus_scope":  {Type: "string"},
			"kinds":         {Type: "array", Items: &core.Schema{Type: "string"}},
			"max_proposals": {Type: "integer"},
		}, "symbol_scope", "corpus_scope"),
		structuredObjectSchema(map[string]*core.Schema{
			"proposals": {
				Type:  "array",
				Items: &core.Schema{Type: "object"},
			},
			"count": {Type: "integer"},
		}, "proposals", "count"),
		map[string]any{
			"relurpic_capability": true,
			"workflow":            "pattern-detect",
		},
		[]core.RiskClass{core.RiskClassReadOnly},
		[]core.EffectClass{core.EffectClassContextInsertion},
	)
}

func (h patternDetectorDetectCapabilityHandler) Invoke(ctx context.Context, _ *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	symbolScope := stringArg(args["symbol_scope"])
	if symbolScope == "" {
		return nil, fmt.Errorf("symbol_scope required")
	}
	corpusScope := stringArg(args["corpus_scope"])
	if corpusScope == "" {
		return nil, fmt.Errorf("corpus_scope required")
	}

	scope, err := resolveSymbolScope(ctx, symbolScope, h.indexManager, h.registry)
	if err != nil {
		return nil, err
	}
	if len(scope.Excerpts) == 0 {
		return nil, fmt.Errorf("no indexed excerpts found for scope %q", symbolScope)
	}

	kinds := normalizePatternKinds(args["kinds"])
	maxProposals := intArg(args["max_proposals"], 5)
	if maxProposals <= 0 {
		maxProposals = 5
	}

	knownTerms, err := h.lookupKnownTerms(ctx, scope, corpusScope)
	if err != nil {
		return nil, err
	}

	prompt := buildPatternDetectionPrompt(scope, corpusScope, kinds, knownTerms, maxProposals)
	resp, err := h.model.Generate(ctx, prompt, &core.LLMOptions{
		Model:       modelName(h.config),
		Temperature: 0.2,
		MaxTokens:   1200,
	})
	if err != nil {
		return nil, err
	}

	parsed, err := parsePatternDetectorResponse(resp.Text)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	proposals := make([]patterns.PatternProposal, 0, len(parsed))
	allowedKinds := make(map[patterns.PatternKind]struct{}, len(kinds))
	for _, kind := range kinds {
		allowedKinds[kind] = struct{}{}
	}
	for idx, item := range parsed {
		instances := make([]patterns.PatternInstance, 0, len(item.Instances))
		for _, instance := range item.Instances {
			filePath := strings.TrimSpace(instance.FilePath)
			if filePath == "" {
				filePath = scope.PrimaryFile()
			}
			inst := patterns.PatternInstance{
				FilePath:  filePath,
				StartLine: instance.StartLine,
				EndLine:   instance.EndLine,
				Excerpt:   strings.TrimSpace(instance.Excerpt),
			}
			if h.graphDB != nil {
				inst.SymbolID = enrichPatternInstanceSymbolID(h.graphDB, inst)
			}
			instances = append(instances, inst)
		}
		proposal := patterns.PatternProposal{
			ID:           patternProposalID(symbolScope, item.Kind, item.Title, idx),
			Kind:         normalizePatternKind(item.Kind),
			Title:        strings.TrimSpace(item.Title),
			Description:  strings.TrimSpace(item.Description),
			Instances:    instances,
			Confidence:   clampConfidence(item.Confidence),
			CorpusScope:  corpusScope,
			CorpusSource: "workspace",
			CreatedAt:    now,
		}
		if proposal.Title == "" || proposal.Description == "" {
			continue
		}
		if _, ok := allowedKinds[proposal.Kind]; !ok {
			continue
		}
		proposals = append(proposals, proposal)
		if len(proposals) >= maxProposals {
			break
		}
	}

	if h.patternStore != nil {
		for _, proposal := range proposals {
			if err := h.patternStore.Save(ctx, patterns.PatternRecord{
				ID:           proposal.ID,
				Kind:         proposal.Kind,
				Title:        proposal.Title,
				Description:  proposal.Description,
				Status:       patterns.PatternStatusProposed,
				Instances:    proposal.Instances,
				CorpusScope:  proposal.CorpusScope,
				CorpusSource: proposal.CorpusSource,
				Confidence:   proposal.Confidence,
				CreatedAt:    proposal.CreatedAt,
				UpdatedAt:    proposal.CreatedAt,
			}); err != nil {
				return nil, err
			}
		}
	}

	return &core.CapabilityExecutionResult{
		Success: true,
		Data: map[string]any{
			"proposals": proposalsAsAny(proposals),
			"count":     len(proposals),
		},
	}, nil
}

type resolvedSymbolScope struct {
	Input     string
	FilePaths []string
	SymbolIDs []string
	Excerpts  []resolvedExcerpt
}

type resolvedExcerpt struct {
	FilePath  string
	StartLine int
	EndLine   int
	Content   string
}

func (r resolvedSymbolScope) PrimaryFile() string {
	if len(r.FilePaths) == 0 {
		return ""
	}
	return r.FilePaths[0]
}

func excerptForFile(ctx context.Context, registry *capability.Registry, path string) (resolvedExcerpt, error) {
	data, err := readWorkspaceFile(ctx, registry, path)
	if err != nil {
		return resolvedExcerpt{}, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	end := len(lines)
	if end == 0 {
		end = 1
	}
	return resolvedExcerpt{
		FilePath:  path,
		StartLine: 1,
		EndLine:   end,
		Content:   strings.TrimSpace(strings.Join(lines, "\n")),
	}, nil
}

func excerptForLines(ctx context.Context, registry *capability.Registry, path string, startLine, endLine int) (resolvedExcerpt, error) {
	data, err := readWorkspaceFile(ctx, registry, path)
	if err != nil {
		return resolvedExcerpt{}, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if startLine <= 0 {
		startLine = 1
	}
	if endLine < startLine {
		endLine = startLine
	}
	if startLine > len(lines) {
		startLine = len(lines)
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine <= 0 {
		startLine = 1
	}
	if endLine <= 0 {
		endLine = 1
	}
	content := ""
	if len(lines) > 0 && startLine-1 < len(lines) && endLine <= len(lines) {
		content = strings.Join(lines[startLine-1:endLine], "\n")
	}
	return resolvedExcerpt{
		FilePath:  path,
		StartLine: startLine,
		EndLine:   endLine,
		Content:   strings.TrimSpace(content),
	}, nil
}

func appendNodeIDs(base []string, nodes []*ast.Node) []string {
	seen := make(map[string]struct{}, len(base))
	for _, id := range base {
		seen[id] = struct{}{}
	}
	for _, node := range nodes {
		if node == nil || node.ID == "" {
			continue
		}
		if _, ok := seen[node.ID]; ok {
			continue
		}
		base = append(base, node.ID)
		seen[node.ID] = struct{}{}
	}
	sort.Strings(base)
	return base
}

func readWorkspaceFile(ctx context.Context, registry *capability.Registry, path string) ([]byte, error) {
	if registry == nil {
		return nil, fmt.Errorf("capability registry required for file access")
	}
	result, err := registry.InvokeCapability(ctx, core.NewContext(), "file_read", map[string]any{"path": path})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("file_read returned no result for %s", path)
	}
	if !result.Success {
		msg := strings.TrimSpace(result.Error)
		if msg == "" {
			msg = fmt.Sprintf("file_read failed for %s", path)
		}
		return nil, fmt.Errorf("%s", msg)
	}
	content, _ := result.Data["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("file_read returned no content for %s", path)
	}
	return []byte(content), nil
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

var scopeTokenPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

func (h patternDetectorDetectCapabilityHandler) lookupKnownTerms(ctx context.Context, scope resolvedSymbolScope, corpusScope string) ([]retrieval.AnchorRef, error) {
	if h.retrievalDB == nil {
		return nil, nil
	}
	termSet := make(map[string]struct{})
	for _, excerpt := range scope.Excerpts {
		for _, token := range scopeTokenPattern.FindAllString(excerpt.Content, -1) {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			termSet[token] = struct{}{}
			if len(termSet) >= 32 {
				break
			}
		}
		if len(termSet) >= 32 {
			break
		}
	}
	terms := sortedKeys(termSet)
	if len(terms) == 0 {
		return nil, nil
	}
	return retrieval.AnchorsForTerms(ctx, h.retrievalDB, terms, corpusScope)
}

func normalizePatternKinds(raw any) []patterns.PatternKind {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 {
		return []patterns.PatternKind{
			patterns.PatternKindStructural,
			patterns.PatternKindSemantic,
			patterns.PatternKindBehavioral,
			patterns.PatternKindBoundary,
		}
	}
	out := make([]patterns.PatternKind, 0, len(values))
	seen := make(map[patterns.PatternKind]struct{}, len(values))
	for _, value := range values {
		kind := normalizePatternKind(fmt.Sprint(value))
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		out = append(out, kind)
	}
	if len(out) == 0 {
		return normalizePatternKinds(nil)
	}
	return out
}

func normalizePatternKind(raw string) patterns.PatternKind {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(patterns.PatternKindSemantic):
		return patterns.PatternKindSemantic
	case string(patterns.PatternKindBehavioral):
		return patterns.PatternKindBehavioral
	case string(patterns.PatternKindBoundary):
		return patterns.PatternKindBoundary
	default:
		return patterns.PatternKindStructural
	}
}

func intArg(raw any, defaultValue int) int {
	switch typed := raw.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		value, err := typed.Int64()
		if err == nil {
			return int(value)
		}
	}
	return defaultValue
}

func stringArg(raw any) string {
	if raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

type llmPatternProposal struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Instances   []struct {
		FilePath  string `json:"file_path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
		Excerpt   string `json:"excerpt"`
	} `json:"instances"`
	Confidence float64 `json:"confidence"`
}

func parsePatternDetectorResponse(text string) ([]llmPatternProposal, error) {
	var payload struct {
		Proposals []llmPatternProposal `json:"proposals"`
	}
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "[") {
		var proposals []llmPatternProposal
		if err := json.Unmarshal([]byte(trimmed), &proposals); err != nil {
			return nil, err
		}
		return proposals, nil
	}
	extracted := reactpkg.ExtractJSON(text)
	if err := json.Unmarshal([]byte(extracted), &payload); err == nil && len(payload.Proposals) > 0 {
		return payload.Proposals, nil
	}
	var proposals []llmPatternProposal
	if err := json.Unmarshal([]byte(extracted), &proposals); err != nil {
		return nil, err
	}
	return proposals, nil
}

func proposalsAsAny(proposals []patterns.PatternProposal) []any {
	out := make([]any, 0, len(proposals))
	for _, proposal := range proposals {
		out = append(out, map[string]any{
			"id":            proposal.ID,
			"kind":          proposal.Kind,
			"title":         proposal.Title,
			"description":   proposal.Description,
			"instances":     instancesAsAny(proposal.Instances),
			"confidence":    proposal.Confidence,
			"corpus_scope":  proposal.CorpusScope,
			"corpus_source": proposal.CorpusSource,
			"created_at":    proposal.CreatedAt.Format(time.RFC3339Nano),
		})
	}
	return out
}

func instancesAsAny(instances []patterns.PatternInstance) []any {
	out := make([]any, 0, len(instances))
	for _, instance := range instances {
		out = append(out, map[string]any{
			"file_path":  instance.FilePath,
			"start_line": instance.StartLine,
			"end_line":   instance.EndLine,
			"excerpt":    instance.Excerpt,
			"symbol_id":  instance.SymbolID,
		})
	}
	return out
}

func buildPatternDetectionPrompt(scope resolvedSymbolScope, corpusScope string, kinds []patterns.PatternKind, anchors []retrieval.AnchorRef, maxProposals int) string {
	var b strings.Builder
	b.WriteString("You are a code pattern detector.\n")
	b.WriteString("Inspect the provided scope and return grounded pattern proposals only.\n")
	b.WriteString("Respond with valid JSON: {\"proposals\":[{\"kind\":\"structural|semantic|behavioral|boundary\",\"title\":\"...\",\"description\":\"...\",\"instances\":[{\"file_path\":\"...\",\"start_line\":1,\"end_line\":2,\"excerpt\":\"...\"}],\"confidence\":0.0}]}\n")
	b.WriteString(fmt.Sprintf("Corpus scope: %s\n", corpusScope))
	b.WriteString(fmt.Sprintf("Kinds: %s\n", joinPatternKinds(kinds)))
	b.WriteString(fmt.Sprintf("Max proposals: %d\n", maxProposals))
	if len(anchors) > 0 {
		b.WriteString("Known terms:\n")
		for _, anchor := range anchors {
			b.WriteString(fmt.Sprintf("- %s: %s\n", anchor.Term, anchor.Definition))
		}
	}
	b.WriteString("Scope excerpts:\n")
	for _, excerpt := range scope.Excerpts {
		b.WriteString(fmt.Sprintf("FILE %s [%d-%d]\n%s\n", filepath.Base(excerpt.FilePath), excerpt.StartLine, excerpt.EndLine, excerpt.Content))
	}
	return b.String()
}

func joinPatternKinds(kinds []patterns.PatternKind) string {
	parts := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		parts = append(parts, string(kind))
	}
	return strings.Join(parts, ", ")
}

func patternProposalID(scope, kind, title string, index int) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%s|%s|%d", scope, kind, title, index)))
	return fmt.Sprintf("pattern-%x", sum[:8])
}

type graphNodeProps struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Name      string `json:"name"`
}

func enrichPatternInstanceSymbolID(graphDB *graphdb.Engine, instance patterns.PatternInstance) string {
	if graphDB == nil || strings.TrimSpace(instance.FilePath) == "" {
		return ""
	}
	nodes := graphDB.NodesBySource(instance.FilePath)
	bestID := ""
	bestSpan := int(^uint(0) >> 1)
	for _, node := range nodes {
		if len(node.Props) == 0 {
			continue
		}
		var props graphNodeProps
		if err := json.Unmarshal(node.Props, &props); err != nil {
			continue
		}
		if props.StartLine == 0 || props.EndLine == 0 {
			continue
		}
		if instance.StartLine < props.StartLine || instance.EndLine > props.EndLine {
			continue
		}
		span := props.EndLine - props.StartLine
		if span < bestSpan {
			bestSpan = span
			bestID = node.ID
		}
	}
	return bestID
}
