// Package ast provides code parsing and AST indexing across multiple languages.
// IndexManager orchestrates incremental symbol detection, language detection, and
// persistence of parsed symbols to an index store for use by search and context tools.
package ast

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

// IndexConfig configures the IndexManager.
type IndexConfig struct {
	WorkspacePath     string
	EnableIncremental bool
	EnableSummaries   bool
	ParallelWorkers   int
	IgnorePatterns    []string
}

// IndexManager orchestrates parsing and memory.
type IndexManager struct {
	store            IndexStore
	parserRegistry   *ParserRegistry
	languageDetector *LanguageDetector
	GraphDB          *graphdb.Engine
	mu               sync.Mutex
	indexing         map[string]bool
	config           IndexConfig
	symbolProvider   DocumentSymbolProvider
	pathFilter       func(path string, isDir bool) bool
	fileScope        *sandbox.FileScopePolicy
	workspaceIndex   workspaceIndexState
}

type workspaceIndexState struct {
	running bool
	ready   bool
	err     error
	readyCh chan struct{}
}

var (
	ErrWorkspaceIndexInProgress = errors.New("workspace index already running")
	errWorkspaceIndexReady      = errors.New("workspace index already ready")
)

// NewIndexManager builds a manager with default parsers.
func NewIndexManager(store IndexStore, config IndexConfig) *IndexManager {
	manager := &IndexManager{
		store:            store,
		parserRegistry:   NewParserRegistry(),
		languageDetector: NewLanguageDetector(),
		indexing:         make(map[string]bool),
		config:           config,
		workspaceIndex: workspaceIndexState{
			readyCh: make(chan struct{}),
		},
	}
	manager.registerDefaultParsers()
	return manager
}

// WorkspacePath returns the configured workspace root for the index manager.
func (im *IndexManager) WorkspacePath() string {
	if im == nil {
		return ""
	}
	return strings.TrimSpace(im.config.WorkspacePath)
}

func (im *IndexManager) registerDefaultParsers() {
	im.RegisterParser(NewGoParser())
	im.RegisterParser(NewMarkdownParser())
}

// RegisterParser makes an additional parser available.
func (im *IndexManager) RegisterParser(parser Parser) {
	if parser == nil {
		return
	}
	im.parserRegistry.Register(parser)
}

// UseSymbolProvider wires an optional document symbol source for fallback
// indexing when language-specific parsers are not available.
func (im *IndexManager) UseSymbolProvider(provider DocumentSymbolProvider) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.symbolProvider = provider
}

// SetPathFilter installs an optional filter that can skip directories/files
// during indexing (e.g. to enforce manifest filesystem permissions).
func (im *IndexManager) SetPathFilter(filter func(path string, isDir bool) bool) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.pathFilter = filter
}

// SetFileScope installs a sandbox file scope that is enforced before files are
// read or indexed.
func (im *IndexManager) SetFileScope(scope *sandbox.FileScopePolicy) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.fileScope = scope
}

// IndexFile parses and stores AST for a file path.
func (im *IndexManager) IndexFile(path string) error {
	if !im.allowedPath(core.FileSystemRead, path, false) {
		return nil
	}
	im.mu.Lock()
	if im.indexing[path] {
		im.mu.Unlock()
		return fmt.Errorf("index already running for %s", path)
	}
	im.indexing[path] = true
	im.mu.Unlock()
	defer func() {
		im.mu.Lock()
		delete(im.indexing, path)
		im.mu.Unlock()
	}()

	language := im.languageDetector.Detect(path)
	category := im.languageDetector.DetectCategory(language)
	parser, ok := im.parserRegistry.GetParser(language)

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	contentHash := HashContent(string(content))

	if existing, err := im.store.GetFileByPath(path); err == nil && existing != nil {
		if existing.ContentHash == contentHash {
			return nil
		}
		if err := im.store.DeleteFile(existing.ID); err != nil {
			return fmt.Errorf("delete previous index: %w", err)
		}
	}

	if !ok {
		return im.indexWithSymbols(path, string(content), language, category, contentHash)
	}

	result, err := parser.Parse(string(content), path)
	if err != nil {
		if symErr := im.indexWithSymbols(path, string(content), language, category, contentHash); symErr == nil {
			return nil
		}
		return err
	}
	return im.persist(result, contentHash)
}

// RefreshFiles incrementally refreshes the AST index for the provided files.
// Missing or now-disallowed files are removed from the index.
func (im *IndexManager) RefreshFiles(paths []string) error {
	if im == nil || len(paths) == 0 {
		return nil
	}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if err := im.refreshFile(path); err != nil {
			return err
		}
	}
	return nil
}

func (im *IndexManager) refreshFile(path string) error {
	if !im.allowedPath(core.FileSystemRead, path, false) {
		return im.removeIndexedFile(path)
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return im.removeIndexedFile(path)
		}
		return err
	}
	return im.IndexFile(path)
}

func (im *IndexManager) removeIndexedFile(path string) error {
	existing, err := im.store.GetFileByPath(path)
	if err != nil || existing == nil {
		return err
	}
	if err := im.store.DeleteFile(existing.ID); err != nil {
		return err
	}
	return im.syncGraphDelete(path)
}

// StartIndexing launches a background workspace index pass unless one is
// already running or the current workspace is already ready.
func (im *IndexManager) StartIndexing(ctx context.Context) error {
	if err := im.beginWorkspaceIndex(); err != nil {
		if errors.Is(err, ErrWorkspaceIndexInProgress) || errors.Is(err, errWorkspaceIndexReady) {
			return nil
		}
		return err
	}
	go func() {
		im.finishWorkspaceIndex(im.runWorkspaceIndex(ctx))
	}()
	return nil
}

// Ready reports whether the most recent workspace index pass completed
// successfully.
func (im *IndexManager) Ready() bool {
	if im == nil {
		return false
	}
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.workspaceIndex.ready
}

// WaitUntilReady blocks until the current workspace index pass completes or
// the caller's context expires. If no workspace index is running, it returns
// the latest terminal state immediately.
func (im *IndexManager) WaitUntilReady(ctx context.Context) error {
	if im == nil {
		return nil
	}
	im.mu.Lock()
	running := im.workspaceIndex.running
	ready := im.workspaceIndex.ready
	err := im.workspaceIndex.err
	readyCh := im.workspaceIndex.readyCh
	im.mu.Unlock()

	if ready || (!running && err == nil) {
		return nil
	}
	if !running {
		return err
	}
	select {
	case <-readyCh:
		im.mu.Lock()
		defer im.mu.Unlock()
		return im.workspaceIndex.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// LastIndexError returns the terminal error from the latest workspace index
// pass, if any.
func (im *IndexManager) LastIndexError() error {
	if im == nil {
		return nil
	}
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.workspaceIndex.err
}

// IndexWorkspace walks the workspace and indexes files.
func (im *IndexManager) IndexWorkspace() error {
	return im.IndexWorkspaceContext(context.Background())
}

// IndexWorkspaceContext walks the workspace and indexes files until completion
// or cancellation.
func (im *IndexManager) IndexWorkspaceContext(ctx context.Context) error {
	if err := im.beginWorkspaceIndex(); err != nil {
		if errors.Is(err, errWorkspaceIndexReady) {
			return nil
		}
		return err
	}
	err := im.runWorkspaceIndex(ctx)
	im.finishWorkspaceIndex(err)
	return err
}

func (im *IndexManager) runWorkspaceIndex(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	root := im.config.WorkspacePath
	if root == "" {
		root = "."
	}
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if !im.allowedPath(core.FileSystemList, path, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if !im.allowedPath(core.FileSystemRead, path, false) {
			return nil
		}
		if im.shouldIgnore(path) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return err
	}
	if im.config.ParallelWorkers > 1 {
		return im.indexFilesParallel(ctx, files)
	}
	return im.indexFilesSequential(ctx, files)
}

func (im *IndexManager) shouldIgnore(path string) bool {
	for _, pattern := range im.config.IgnorePatterns {
		match, err := filepath.Match(pattern, filepath.Base(path))
		if err == nil && match {
			return true
		}
		if strings.Contains(path, pattern) {
			return true
		}
	}
	return false
}

func (im *IndexManager) allowedPath(action core.FileSystemAction, path string, isDir bool) bool {
	im.mu.Lock()
	scope := im.fileScope
	filter := im.pathFilter
	im.mu.Unlock()
	if scope != nil && scope.Check(action, path) != nil {
		return false
	}
	if filter != nil && !filter(path, isDir) {
		return false
	}
	return true
}

func (im *IndexManager) indexFilesSequential(ctx context.Context, files []string) error {
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := im.IndexFile(file); err != nil {
			log.Printf("AST index warning: %v", err)
		}
	}
	return nil
}

func (im *IndexManager) indexFilesParallel(ctx context.Context, files []string) error {
	workerCount := im.config.ParallelWorkers
	if workerCount <= 0 {
		workerCount = 2
	}
	var wg sync.WaitGroup
	fileCh := make(chan string)
	var (
		errMu    sync.Mutex
		firstErr error
	)
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					recordErr(ctx.Err())
					return
				case file, ok := <-fileCh:
					if !ok {
						return
					}
					if err := im.IndexFile(file); err != nil {
						recordErr(fmt.Errorf("%s: %w", file, err))
					}
				}
			}
		}()
	}
	for _, file := range files {
		select {
		case <-ctx.Done():
			recordErr(ctx.Err())
			close(fileCh)
			wg.Wait()
			return ctx.Err()
		case fileCh <- file:
		}
	}
	close(fileCh)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	return firstErr
}

func (im *IndexManager) beginWorkspaceIndex() error {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.workspaceIndex.running {
		return ErrWorkspaceIndexInProgress
	}
	if im.workspaceIndex.ready {
		return errWorkspaceIndexReady
	}
	im.workspaceIndex.running = true
	im.workspaceIndex.ready = false
	im.workspaceIndex.err = nil
	im.workspaceIndex.readyCh = make(chan struct{})
	return nil
}

func (im *IndexManager) finishWorkspaceIndex(err error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if !im.workspaceIndex.running {
		return
	}
	im.workspaceIndex.running = false
	im.workspaceIndex.ready = err == nil
	im.workspaceIndex.err = err
	close(im.workspaceIndex.readyCh)
}

// Close releases any underlying resources owned by the store.
func (im *IndexManager) Close() error {
	if im == nil {
		return nil
	}
	im.mu.Lock()
	running := im.workspaceIndex.running
	readyCh := im.workspaceIndex.readyCh
	im.mu.Unlock()
	if running && readyCh != nil {
		<-readyCh
	}
	var firstErr error
	if im.GraphDB != nil {
		if err := im.GraphDB.Close(); err != nil {
			firstErr = err
		}
	}
	if im.store == nil {
		return firstErr
	}
	closer, ok := im.store.(io.Closer)
	if !ok {
		return firstErr
	}
	if err := closer.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (im *IndexManager) indexWithSymbols(path, content, language string, category Category, contentHash string) error {
	im.mu.Lock()
	provider := im.symbolProvider
	im.mu.Unlock()
	if provider == nil {
		return fmt.Errorf("no parser for %s", language)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	symbols, err := provider.DocumentSymbols(ctx, path)
	if err != nil {
		return err
	}
	if len(symbols) == 0 {
		return fmt.Errorf("symbol provider returned no data for %s", path)
	}
	fileID := GenerateFileID(path)
	lines := strings.Count(content, "\n") + 1
	now := time.Now().UTC()
	rootType := NodeTypeDocument
	if category == CategoryCode {
		rootType = NodeTypePackage
	}
	root := &Node{
		ID:        fmt.Sprintf("%s:symbol-root", fileID),
		FileID:    fileID,
		Type:      rootType,
		Category:  category,
		Language:  language,
		Name:      filepath.Base(path),
		StartLine: 1,
		EndLine:   lines,
		CreatedAt: now,
		UpdatedAt: now,
	}
	nodes := []*Node{root}
	nodes = append(nodes, im.buildSymbolNodes(symbols, root.ID, fileID, category, language, now)...)
	result := &ParseResult{
		RootNode: root,
		Nodes:    nodes,
		Edges:    nil,
		Metadata: &FileMetadata{
			ID:            fileID,
			Path:          path,
			RelativePath:  filepath.Base(path),
			Language:      language,
			Category:      category,
			LineCount:     lines,
			TokenCount:    len(content),
			ContentHash:   contentHash,
			RootNodeID:    root.ID,
			NodeCount:     len(nodes),
			EdgeCount:     0,
			IndexedAt:     now,
			ParserVersion: "lsp_symbols",
		},
	}
	return im.persist(result, contentHash)
}

func (im *IndexManager) buildSymbolNodes(symbols []DocumentSymbol, parentID, fileID string, category Category, language string, timestamp time.Time) []*Node {
	var nodes []*Node
	for _, sym := range symbols {
		nodeType := sym.Kind
		if nodeType == "" {
			nodeType = NodeTypeSection
		}
		start := sym.StartLine
		if start <= 0 {
			start = 1
		}
		end := sym.EndLine
		if end < start {
			end = start
		}
		node := &Node{
			ID:        fmt.Sprintf("%s:symbol:%s:%d", fileID, sanitizeSymbolName(sym.Name), start),
			ParentID:  parentID,
			FileID:    fileID,
			Type:      nodeType,
			Category:  category,
			Language:  language,
			Name:      sym.Name,
			StartLine: start,
			EndLine:   end,
			CreatedAt: timestamp,
			UpdatedAt: timestamp,
		}
		nodes = append(nodes, node)
		if len(sym.Children) > 0 {
			nodes = append(nodes, im.buildSymbolNodes(sym.Children, node.ID, fileID, category, language, timestamp)...)
		}
	}
	return nodes
}

func sanitizeSymbolName(name string) string {
	if name == "" {
		return "symbol"
	}
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, ":", "_")
	return strings.ToLower(name)
}

func (im *IndexManager) persist(result *ParseResult, contentHash string) error {
	if result.Metadata == nil {
		return fmt.Errorf("parse result missing metadata")
	}
	if result.Metadata.ContentHash == "" {
		result.Metadata.ContentHash = contentHash
	}
	tx, err := im.store.BeginTransaction()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := im.store.SaveFile(result.Metadata); err != nil {
		return err
	}
	if err := tx.SaveNodes(result.Nodes); err != nil {
		return err
	}
	if err := tx.SaveEdges(result.Edges); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return im.syncGraphResult(result)
}

// QuerySymbol looks up nodes whose name matches the pattern.
func (im *IndexManager) QuerySymbol(pattern string) ([]*Node, error) {
	return im.store.SearchNodes(NodeQuery{
		NamePattern: pattern,
		Limit:       100,
	})
}

// SearchNodes routes to the underlying store.
func (im *IndexManager) SearchNodes(query NodeQuery) ([]*Node, error) {
	return im.store.SearchNodes(query)
}

// CallGraph summarizes direct callers/callees.
type CallGraph struct {
	Root    *Node
	Callees map[string][]*Node
	Callers map[string][]*Node
}

// GetCallGraph returns the call relationships for the identified symbol.
func (im *IndexManager) GetCallGraph(symbol string) (*CallGraph, error) {
	nodes, err := im.QuerySymbol(symbol)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("symbol not found: %s", symbol)
	}
	root := nodes[0]
	callees, err := im.store.GetCallees(root.ID)
	if err != nil {
		return nil, err
	}
	callers, err := im.store.GetCallers(root.ID)
	if err != nil {
		return nil, err
	}
	return &CallGraph{
		Root:    root,
		Callees: map[string][]*Node{root.ID: callees},
		Callers: map[string][]*Node{root.ID: callers},
	}, nil
}

// DependencyGraph expresses dependencies and dependents.
type DependencyGraph struct {
	Root         *Node
	Dependencies []*Node
	Dependents   []*Node
}

// GetDependencyGraph resolves dependencies for a symbol.
func (im *IndexManager) GetDependencyGraph(symbol string) (*DependencyGraph, error) {
	nodes, err := im.QuerySymbol(symbol)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("symbol not found: %s", symbol)
	}
	root := nodes[0]
	deps, err := im.store.GetDependencies(root.ID)
	if err != nil {
		return nil, err
	}
	dependents, err := im.store.GetDependents(root.ID)
	if err != nil {
		return nil, err
	}
	return &DependencyGraph{
		Root:         root,
		Dependencies: deps,
		Dependents:   dependents,
	}, nil
}

// Stats proxies store.GetStats for callers.
func (im *IndexManager) Stats() (*IndexStats, error) {
	return im.store.GetStats()
}

// LastIndexedAt fetches the timestamp recorded for a path, if any.
func (im *IndexManager) LastIndexedAt(path string) (time.Time, error) {
	file, err := im.store.GetFileByPath(path)
	if err != nil {
		return time.Time{}, err
	}
	if file == nil {
		return time.Time{}, nil
	}
	return file.IndexedAt, nil
}

// Store exposes the underlying IndexStore for advanced queries.
func (im *IndexManager) Store() IndexStore {
	if im == nil {
		return nil
	}
	return im.store
}

func (im *IndexManager) syncGraphDelete(path string) error {
	if im == nil || im.GraphDB == nil || path == "" {
		return nil
	}
	nodes := im.GraphDB.NodesBySource(path)
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return im.GraphDB.DeleteNodes(ids)
}

func (im *IndexManager) syncGraphResult(result *ParseResult) error {
	if im == nil || im.GraphDB == nil || result == nil || result.Metadata == nil {
		return nil
	}
	sourcePath := result.Metadata.Path
	if sourcePath == "" {
		return nil
	}
	existing := im.GraphDB.NodesBySource(sourcePath)
	deleteIDs := make([]string, 0, len(existing))
	for _, node := range existing {
		deleteIDs = append(deleteIDs, node.ID)
	}
	if err := im.GraphDB.DeleteNodes(deleteIDs); err != nil {
		return err
	}
	nodeBatch := make([]graphdb.NodeRecord, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		record, ok := graphNodeRecord(node, sourcePath)
		if !ok {
			continue
		}
		nodeBatch = append(nodeBatch, record)
	}
	if err := im.GraphDB.UpsertNodes(nodeBatch); err != nil {
		return err
	}
	edgeBatch := make([]graphdb.EdgeRecord, 0, len(result.Edges)*2+len(result.Nodes)*2)
	for _, edge := range result.Edges {
		kind, inverse, ok := graphEdgeKinds(edge.Type)
		if !ok {
			continue
		}
		records, err := graphEdgeRecords(edge.SourceID, edge.TargetID, kind, inverse, 1, edge.Attributes)
		if err != nil {
			return err
		}
		edgeBatch = append(edgeBatch, records...)
	}
	for _, node := range result.Nodes {
		if node == nil || node.ParentID == "" {
			continue
		}
		edgeBatch = append(edgeBatch,
			graphdb.EdgeRecord{SourceID: node.ParentID, TargetID: node.ID, Kind: EdgeKindContains, Weight: 1},
			graphdb.EdgeRecord{SourceID: node.ID, TargetID: node.ParentID, Kind: EdgeKindContainedBy, Weight: 1},
		)
	}
	return im.GraphDB.LinkEdges(edgeBatch)
}
