package contextmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/perfstats"
	"github.com/lexcodex/relurpify/framework/search"
	platformast "github.com/lexcodex/relurpify/platform/ast"
)

const astIndexReadyTimeout = 2 * time.Second

// ProgressiveLoader manages incremental context loading.
type ProgressiveLoader struct {
	contextManager *ContextManager
	indexManager   *ast.IndexManager
	searchEngine   *search.SearchEngine
	memoryStore    memory.MemoryStore
	summarizer     core.Summarizer
	budget         *core.ContextBudget

	loadHistory []ContextLoadEvent
	loadedFiles map[string]DetailLevel
	fileCache   map[string]cachedFileContent
	viewCache   map[string]cachedFileViews
}

type cachedFileContent struct {
	content string
	version string
}

type cachedFileViews struct {
	version    string
	summary    string
	detailed   string
	signatures string
}

// NewProgressiveLoader builds a loader with optional helpers.
func NewProgressiveLoader(
	contextManager *ContextManager,
	indexManager *ast.IndexManager,
	searchEngine *search.SearchEngine,
	memoryStore memory.MemoryStore,
	budget *core.ContextBudget,
	summarizer core.Summarizer,
) *ProgressiveLoader {
	return &ProgressiveLoader{
		contextManager: contextManager,
		indexManager:   indexManager,
		searchEngine:   searchEngine,
		memoryStore:    memoryStore,
		budget:         budget,
		summarizer:     summarizer,
		loadHistory:    make([]ContextLoadEvent, 0),
		loadedFiles:    make(map[string]DetailLevel),
		fileCache:      make(map[string]cachedFileContent),
		viewCache:      make(map[string]cachedFileViews),
	}
}

// InitialLoad executes the strategy's first context request.
func (pl *ProgressiveLoader) InitialLoad(task *core.Task, strategy ContextStrategy) error {
	if pl == nil || strategy == nil {
		return fmt.Errorf("progressive loader not initialized")
	}
	request, err := strategy.SelectContext(task, pl.budget)
	if err != nil {
		return fmt.Errorf("select context: %w", err)
	}
	if loader, ok := strategy.(ChunkLoader); ok {
		chunks, err := loader.LoadChunks(task, pl.budget)
		if err != nil {
			return fmt.Errorf("load chunks: %w", err)
		}
		request.ChunkSequence = append(request.ChunkSequence, chunks...)
	}
	ResolveContextRequestPaths(request, task)
	return pl.ExecuteContextRequest(request, "initial")
}

// ExecuteContextRequest loads the requested artifacts.
func (pl *ProgressiveLoader) ExecuteContextRequest(request *ContextRequest, trigger string) error {
	if request == nil {
		return nil
	}
	event := ContextLoadEvent{
		Timestamp: time.Now(),
		Trigger:   trigger,
		Success:   true,
	}
	for _, chunk := range request.ChunkSequence {
		if err := pl.loadChunk(chunk); err != nil {
			event.Success = false
			event.Reason = err.Error()
			continue
		}
		event.ItemsLoaded++
	}
	for _, fileReq := range request.Files {
		if err := pl.loadFile(fileReq); err != nil {
			event.Success = false
			event.Reason = err.Error()
			continue
		}
		event.ItemsLoaded++
	}
	for _, astQuery := range request.ASTQueries {
		if err := pl.executeASTQuery(astQuery); err != nil {
			event.Success = false
			event.Reason = err.Error()
			continue
		}
		event.ItemsLoaded++
	}
	for _, searchQuery := range request.SearchQueries {
		if err := pl.executeSearchQuery(searchQuery); err != nil {
			event.Success = false
			event.Reason = err.Error()
			continue
		}
		event.ItemsLoaded++
	}
	for _, memoryQuery := range request.MemoryQueries {
		if err := pl.executeMemoryQuery(memoryQuery); err != nil {
			event.Success = false
			event.Reason = err.Error()
			continue
		}
		event.ItemsLoaded++
	}
	pl.loadHistory = append(pl.loadHistory, event)
	return nil
}

// ExpandContext increases the detail for a file.
func (pl *ProgressiveLoader) ExpandContext(path string, level DetailLevel) error {
	if level > DetailSignatureOnly {
		level = DetailSignatureOnly
	}
	if existing, ok := pl.loadedFiles[path]; ok && existing <= level {
		return nil
	}
	return pl.loadFile(FileRequest{
		Path:        path,
		DetailLevel: level,
		Priority:    0,
	})
}

// DrillDown loads full content for a file.
func (pl *ProgressiveLoader) DrillDown(path string) error {
	return pl.ExpandContext(path, DetailFull)
}

// LoadRelatedFiles fetches dependencies for the target file.
func (pl *ProgressiveLoader) LoadRelatedFiles(path string, depth int) error {
	if pl.indexManager == nil || depth <= 0 {
		return nil
	}
	nodes, err := pl.indexManager.QuerySymbol(filepath.Base(path))
	if err != nil || len(nodes) == 0 {
		return err
	}
	deps, err := pl.indexManager.Store().GetDependencies(nodes[0].ID)
	if err != nil {
		return err
	}
	for _, dep := range deps {
		if dep == nil || dep.FileID == "" {
			continue
		}
		fileMeta, err := pl.indexManager.Store().GetFile(dep.FileID)
		if err != nil || fileMeta == nil {
			continue
		}
		_ = pl.loadFile(FileRequest{
			Path:        fileMeta.Path,
			DetailLevel: DetailConcise,
			Priority:    1,
		})
	}
	return nil
}

func (pl *ProgressiveLoader) loadFile(req FileRequest) error {
	if pl.contextManager == nil {
		return fmt.Errorf("context manager unavailable")
	}
	if req.Path == "" {
		return fmt.Errorf("file path required")
	}
	prevLevel, reread := pl.loadedFiles[req.Path]
	demotion := reread && req.DetailLevel != DetailFull
	content, refreshed, err := pl.rawFileContent(req.Path)
	if err != nil {
		return err
	}
	perfstats.IncProgressiveFileRead(reread && refreshed, demotion && refreshed)
	item := pl.buildFileContextItem(req, content)

	// Stamp detail_demotion derivation if demoting to a lower detail level
	if demotion {
		loss := lossForDetailDemotion(prevLevel, req.DetailLevel)
		if existing := pl.fileItem(req.Path); existing != nil {
			var chain *core.DerivationChain
			if existing.Derivation() != nil {
				chain = existing.Derivation()
			} else {
				origin := core.OriginDerivation("contextmgr")
				chain = &origin
			}
			detail := fmt.Sprintf("demotion %s → %s", prevLevel, req.DetailLevel)
			derived := chain.Derive("detail_demotion", "contextmgr", loss, detail)
			item = item.WithDerivation(derived).(*core.FileContextItem)
		}
	}

	if err := pl.contextManager.UpsertFileItem(item); err != nil {
		return fmt.Errorf("add file to context: %w", err)
	}
	pl.loadedFiles[req.Path] = req.DetailLevel
	return nil
}

func (pl *ProgressiveLoader) loadChunk(chunk ContextChunk) error {
	if pl.contextManager == nil {
		return fmt.Errorf("context manager unavailable")
	}
	ref := &core.ContextReference{
		Kind:    core.ContextReferenceWorkflowArtifact,
		ID:      chunk.ID,
		Detail:  "bkc_chunk",
		Version: chunk.Metadata["version"],
	}
	if len(chunk.Metadata) > 0 {
		ref.Metadata = make(map[string]string, len(chunk.Metadata))
		for key, value := range chunk.Metadata {
			ref.Metadata[key] = value
		}
	}
	item := &core.MemoryContextItem{
		Source:       "semantic_chunk:" + chunk.ID,
		Content:      chunk.Content,
		Reference:    ref,
		LastAccessed: time.Now().UTC(),
		Relevance:    0.95,
		PriorityVal:  0,
	}
	return pl.contextManager.AddItem(item)
}

// DemoteToFree progressively lowers detail on less-important files before pruning.
func (pl *ProgressiveLoader) DemoteToFree(targetTokens int, protected map[string]struct{}) (int, error) {
	if pl == nil || pl.contextManager == nil || targetTokens <= 0 {
		return 0, nil
	}
	type candidate struct {
		item  *core.FileContextItem
		score float64
	}
	var candidates []candidate
	for _, item := range pl.contextManager.FileItems() {
		if item == nil || item.Pinned || item.Priority() == 0 {
			continue
		}
		if _, ok := protected[item.Path]; ok {
			continue
		}
		level, ok := pl.loadedFiles[item.Path]
		if !ok {
			level = inferredDetailLevel(item)
		}
		if _, ok := nextLessDetailedLevel(level); !ok {
			continue
		}
		score := float64(item.TokenCount()) * (1.0 - item.RelevanceScore() + 0.2) * float64(item.Priority()+1)
		score += item.Age().Minutes() / 10.0
		candidates = append(candidates, candidate{item: item, score: score})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].item.Path < candidates[j].item.Path
		}
		return candidates[i].score > candidates[j].score
	})

	freed := 0
	for _, candidate := range candidates {
		current := pl.loadedFiles[candidate.item.Path]
		for {
			next, ok := nextLessDetailedLevel(current)
			if !ok {
				break
			}
			before := 0
			if existing := pl.fileItem(candidate.item.Path); existing != nil {
				before = existing.TokenCount()
			}
			if err := pl.loadFile(FileRequest{
				Path:        candidate.item.Path,
				DetailLevel: next,
				Priority:    candidate.item.PriorityVal,
				Pinned:      candidate.item.Pinned,
			}); err != nil {
				break
			}
			after := 0
			if updated := pl.fileItem(candidate.item.Path); updated != nil {
				after = updated.TokenCount()
			}
			current = next
			if before > after {
				freed += before - after
			}
			if freed >= targetTokens {
				break
			}
			if before != after {
				break
			}
		}
		if freed >= targetTokens {
			break
		}
	}
	if freed == 0 {
		return 0, fmt.Errorf("no file context available for demotion")
	}
	return freed, nil
}

func (pl *ProgressiveLoader) applyDetailLevel(content, path string, level DetailLevel) string {
	switch level {
	case DetailFull:
		return content
	case DetailDetailed:
		if data := pl.formatDetailed(path); data != "" {
			return data
		}
		return content
	case DetailConcise:
		if summary := pl.fileSummary(path, content); summary != "" {
			return summary
		}
		if len(content) > 500 {
			return content[:500] + "..."
		}
		return content
	case DetailMinimal:
		if summary := pl.fileStats(path); summary != "" {
			return summary
		}
		return filepath.Base(path)
	case DetailSignatureOnly:
		if data := pl.formatSignaturesOnly(path); data != "" {
			return data
		}
		return filepath.Base(path)
	default:
		return content
	}
}

func (pl *ProgressiveLoader) buildFileContextItem(req FileRequest, content string) *core.FileContextItem {
	summary := pl.fileSummary(req.Path, content)
	if summary == "" {
		summary = pl.fileStats(req.Path)
	}
	processed := pl.applyDetailLevel(content, req.Path, req.DetailLevel)
	if summary == "" && processed != content {
		summary = processed
	}
	relevance := 1.0
	if existing := pl.fileItem(req.Path); existing != nil {
		relevance = existing.Relevance
		if req.Priority == 0 && existing.PriorityVal != 0 {
			req.Priority = existing.PriorityVal
		}
		if !req.Pinned {
			req.Pinned = existing.Pinned
		}
	}
	return &core.FileContextItem{
		Path:    req.Path,
		Content: processed,
		Summary: summary,
		Reference: &core.ContextReference{
			Kind:   core.ContextReferenceFile,
			ID:     req.Path,
			URI:    req.Path,
			Detail: detailLevelLabel(req.DetailLevel),
			Metadata: map[string]string{
				"priority": strconv.Itoa(req.Priority),
			},
		},
		LastAccessed: time.Now(),
		Relevance:    relevance,
		PriorityVal:  req.Priority,
		Pinned:       req.Pinned,
	}
}

func (pl *ProgressiveLoader) fileItem(path string) *core.FileContextItem {
	if pl == nil || pl.contextManager == nil || path == "" {
		return nil
	}
	for _, item := range pl.contextManager.FileItems() {
		if item != nil && item.Path == path {
			return item
		}
	}
	return nil
}

func nextLessDetailedLevel(level DetailLevel) (DetailLevel, bool) {
	switch level {
	case DetailFull:
		return DetailDetailed, true
	case DetailDetailed:
		return DetailConcise, true
	case DetailConcise:
		return DetailMinimal, true
	case DetailMinimal:
		return DetailSignatureOnly, true
	default:
		return DetailSignatureOnly, false
	}
}

func inferredDetailLevel(item *core.FileContextItem) DetailLevel {
	if item == nil {
		return DetailMinimal
	}
	switch {
	case item.Content != "" && item.Summary != "" && len(item.Content) <= len(item.Summary):
		return DetailConcise
	case item.Content != "" && len(item.Content) < 256:
		return DetailMinimal
	case item.Content == "" && item.Summary != "":
		return DetailMinimal
	default:
		return DetailDetailed
	}
}

func (pl *ProgressiveLoader) formatDetailed(path string) string {
	if cached := pl.cachedViews(path); cached != nil && cached.detailed != "" {
		return cached.detailed
	}
	nodes := pl.nodesForFile(path)
	if len(nodes) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Type == ast.NodeTypeFunction || node.Type == ast.NodeTypeMethod {
			sb.WriteString(node.Signature)
			sb.WriteString("\n")
			if node.DocString != "" {
				sb.WriteString("  // ")
				sb.WriteString(node.DocString)
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}
	formatted := sb.String()
	pl.updateCachedViews(path, func(views *cachedFileViews) {
		views.detailed = formatted
	})
	return formatted
}

func (pl *ProgressiveLoader) formatSignaturesOnly(path string) string {
	if cached := pl.cachedViews(path); cached != nil && cached.signatures != "" {
		return cached.signatures
	}
	nodes := pl.nodesForFile(path)
	if len(nodes) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, node := range nodes {
		if node == nil {
			continue
		}
		switch node.Type {
		case ast.NodeTypeFunction, ast.NodeTypeMethod, ast.NodeTypeClass:
			sb.WriteString("- ")
			sb.WriteString(node.Name)
			sb.WriteString("\n")
		}
	}
	formatted := sb.String()
	pl.updateCachedViews(path, func(views *cachedFileViews) {
		views.signatures = formatted
	})
	return formatted
}

func (pl *ProgressiveLoader) nodesForFile(path string) []*ast.Node {
	if pl.indexManager == nil {
		return nil
	}
	meta, err := pl.indexManager.Store().GetFileByPath(path)
	if err != nil || meta == nil {
		return nil
	}
	nodes, err := pl.indexManager.Store().GetNodesByFile(meta.ID)
	if err != nil {
		return nil
	}
	return nodes
}

func (pl *ProgressiveLoader) fileSummary(path, content string) string {
	if pl.indexManager == nil {
		if content == "" || pl.summarizer == nil {
			return ""
		}
		return pl.summarizeFileContent(path, content)
	}
	meta, err := pl.indexManager.Store().GetFileByPath(path)
	if err != nil || meta == nil {
		if content == "" || pl.summarizer == nil {
			return ""
		}
		return pl.summarizeFileContent(path, content)
	}
	if meta.Summary != "" {
		return meta.Summary
	}
	if content != "" && pl.summarizer != nil {
		return pl.summarizeFileContent(path, content)
	}
	return ""
}

func (pl *ProgressiveLoader) rawFileContent(path string) (string, bool, error) {
	if path == "" {
		return "", false, fmt.Errorf("file path required")
	}
	version, err := fileVersion(path)
	if err != nil {
		if cached, ok := pl.fileCache[path]; ok {
			return cached.content, false, nil
		}
		return "", false, err
	}
	if cached, ok := pl.fileCache[path]; ok && cached.version == version {
		return cached.content, false, nil
	}
	content, err := ReadFile(path)
	if err != nil {
		return "", false, err
	}
	if cached, ok := pl.fileCache[path]; ok && cached.version != version {
		delete(pl.viewCache, path)
	}
	pl.fileCache[path] = cachedFileContent{
		content: content,
		version: version,
	}
	return content, true, nil
}

func fileVersion(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d", info.ModTime().UTC().UnixNano(), info.Size()), nil
}

func (pl *ProgressiveLoader) summarizeFileContent(path, content string) string {
	if content == "" || pl.summarizer == nil {
		return ""
	}
	if cached := pl.cachedViews(path); cached != nil && cached.summary != "" {
		return cached.summary
	}
	summary, err := pl.summarizer.Summarize(content, core.SummaryConcise)
	if err != nil {
		return ""
	}
	pl.updateCachedViews(path, func(views *cachedFileViews) {
		views.summary = summary
	})
	return summary
}

func (pl *ProgressiveLoader) cachedViews(path string) *cachedFileViews {
	if path == "" {
		return nil
	}
	fileCache, ok := pl.fileCache[path]
	if !ok {
		return nil
	}
	views, ok := pl.viewCache[path]
	if !ok || views.version != fileCache.version {
		return nil
	}
	return &views
}

func (pl *ProgressiveLoader) updateCachedViews(path string, mutate func(*cachedFileViews)) {
	if path == "" || mutate == nil {
		return
	}
	fileCache, ok := pl.fileCache[path]
	if !ok {
		return
	}
	views := pl.viewCache[path]
	if views.version != fileCache.version {
		views = cachedFileViews{version: fileCache.version}
	}
	mutate(&views)
	pl.viewCache[path] = views
}

func (pl *ProgressiveLoader) fileStats(path string) string {
	if pl.indexManager == nil {
		return ""
	}
	meta, err := pl.indexManager.Store().GetFileByPath(path)
	if err != nil || meta == nil {
		return ""
	}
	return fmt.Sprintf("%s: %d lines, %d tokens", filepath.Base(path), meta.LineCount, meta.TokenCount)
}

func (pl *ProgressiveLoader) executeASTQuery(query ASTQuery) error {
	if pl.indexManager == nil {
		return fmt.Errorf("ast index unavailable")
	}
	if err := pl.waitForASTIndexReady(astIndexReadyTimeout); err != nil {
		return err
	}
	tool := platformast.NewASTTool(pl.indexManager)
	if tool == nil {
		return fmt.Errorf("cannot create ast tool")
	}
	params := map[string]interface{}{
		"action": string(query.Type),
		"symbol": query.Symbol,
	}
	if len(query.Filter.Types) > 0 {
		params["type"] = string(query.Filter.Types[0])
	}
	if len(query.Filter.Categories) > 0 {
		params["category"] = string(query.Filter.Categories[0])
	}
	if query.Filter.ExportedOnly {
		params["exported_only"] = true
	}
	result, err := tool.Execute(context.Background(), core.NewContext(), params)
	if err != nil {
		return err
	}
	item := &core.ToolResultContextItem{
		ToolName:     "query_ast",
		Result:       result,
		LastAccessed: time.Now(),
		Relevance:    0.8,
		PriorityVal:  1,
	}
	if pl.contextManager != nil {
		return pl.contextManager.AddItem(item)
	}
	return nil
}

func (pl *ProgressiveLoader) waitForASTIndexReady(timeout time.Duration) error {
	if pl == nil || pl.indexManager == nil {
		return nil
	}
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if err := pl.indexManager.WaitUntilReady(ctx); err != nil {
		return fmt.Errorf("wait for ast index readiness: %w", err)
	}
	return nil
}

func (pl *ProgressiveLoader) executeSearchQuery(query SearchQuery) error {
	if pl.searchEngine == nil {
		return nil
	}
	searchQuery := search.SearchQuery{
		Text:         query.Text,
		Mode:         query.Mode,
		FilePatterns: query.FilePatterns,
		MaxResults:   query.MaxResults,
	}
	results, err := pl.searchEngine.Search(searchQuery)
	if err != nil {
		return err
	}
	for i, result := range results {
		if i >= query.MaxResults {
			break
		}
		_ = pl.loadFile(FileRequest{
			Path:        result.File,
			DetailLevel: DetailConcise,
			Priority:    1,
		})
	}
	return nil
}

func (pl *ProgressiveLoader) executeMemoryQuery(query MemoryQuery) error {
	if pl.memoryStore == nil || pl.contextManager == nil {
		return nil
	}
	maxResults := query.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}
	results, err := pl.memoryStore.Search(context.Background(), query.Query, query.Scope)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("Relevant agent memories:\n")
	for i, result := range results {
		if i >= maxResults {
			break
		}
		data, err := json.Marshal(result.Value)
		if err != nil {
			data = []byte("{}")
		}
		fmt.Fprintf(&sb, "- %s: %s\n", result.Key, data)
	}
	return pl.contextManager.AddItem(&core.MemoryContextItem{
		Source:  "memory:" + query.Query,
		Content: sb.String(),
		Summary: fmt.Sprintf("memory query %q (%d results)", query.Query, minInt(len(results), maxResults)),
		Reference: &core.ContextReference{
			Kind:   core.ContextReferenceRuntimeMemory,
			ID:     query.Query,
			Detail: "query-results",
			Metadata: map[string]string{
				"max_results": strconv.Itoa(maxResults),
			},
		},
		LastAccessed: time.Now().UTC(),
		Relevance:    0.9,
		PriorityVal:  1,
	})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func detailLevelLabel(level DetailLevel) string {
	switch level {
	case DetailFull:
		return "full"
	case DetailDetailed:
		return "detailed"
	case DetailConcise:
		return "concise"
	case DetailMinimal:
		return "minimal"
	case DetailSignatureOnly:
		return "signature"
	default:
		return "unknown"
	}
}

// lossForDetailDemotion calculates information loss magnitude for demoting from one detail level to another
func lossForDetailDemotion(from, to DetailLevel) float64 {
	switch {
	case from == DetailFull && to == DetailDetailed:
		return 0.15
	case from == DetailFull && to == DetailConcise:
		return 0.3
	case from == DetailFull && to == DetailMinimal:
		return 0.5
	case from == DetailFull && to == DetailSignatureOnly:
		return 0.6
	case from == DetailDetailed && to == DetailConcise:
		return 0.3
	case from == DetailDetailed && to == DetailMinimal:
		return 0.5
	case from == DetailDetailed && to == DetailSignatureOnly:
		return 0.6
	case from == DetailConcise && to == DetailMinimal:
		return 0.5
	case from == DetailConcise && to == DetailSignatureOnly:
		return 0.6
	case from == DetailMinimal && to == DetailSignatureOnly:
		return 0.6
	default:
		return 0.0
	}
}
