package pretask

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lexcodex/relurpify/framework/ast"
)

// AnchorExtractor extracts deterministic structural signals from a query.
// It does not call the LLM. All results are confirmed against the AST index.
type AnchorExtractor struct {
	index  IndexQuerier // interface over ast.IndexManager
	config AnchorConfig
}

type AnchorConfig struct {
	// MinSymbolLength filters out very short tokens (default 3).
	MinSymbolLength int
	// MaxSymbols caps how many symbols to confirm against the index (default 12).
	MaxSymbols int
}

// IndexQuerier is the narrow interface the extractor needs from ast.IndexManager.
// Using an interface makes this unit-testable without a real index.
type IndexQuerier interface {
	QuerySymbol(pattern string) ([]*ast.Node, error)
	SearchNodes(query ast.NodeQuery) ([]*ast.Node, error)
}

// Extract builds an AnchorSet from the full pipeline input.
//
// Extraction order (priority):
//   1. input.CurrentTurnFiles — user-selected this turn, not yet loaded.
//      Added unconditionally. No index confirmation. These are the user's
//      explicit intent and take precedence over all other signals.
//   2. input.SessionPins — confirmed in prior turns. Also unconditional.
//   3. @-mentioned file paths parsed from input.Query (trust user-provided paths).
//   4. CamelCase identifiers extracted from input.Query, confirmed against index.
//   5. Package-path-style tokens (e.g. "framework/capability"), confirmed against index.
//
// Files that are already loaded (present in ProgressiveLoader.loadedFiles) are
// still included in the AnchorSet — they were confirmed previously and drive
// retrieval expansion. The loader skips re-reading them (cache hit).
func (e *AnchorExtractor) Extract(input PipelineInput) AnchorSet {
	var anchors AnchorSet
	// 1. CurrentTurnFiles highest priority
	anchors.FilePaths = append(anchors.FilePaths, input.CurrentTurnFiles...)
	// 2. SessionPins
	anchors.SessionPins = append(anchors.SessionPins, input.SessionPins...)
	// 3. @-mentions
	atMentions := extractAtMentions(input.Query)
	anchors.FilePaths = append(anchors.FilePaths, atMentions...)
	// 4. CamelCase identifiers
	camelSymbols := extractCamelCase(input.Query)
	// 5. Package paths
	pkgRefs := extractPackagePaths(input.Query)
	// Confirm symbols and package paths via index
	confirmedSymbols := make([]string, 0)
	for _, sym := range camelSymbols {
		if len(sym) < e.config.MinSymbolLength {
			continue
		}
		nodes, err := e.index.QuerySymbol(sym)
		if err == nil && len(nodes) > 0 {
			confirmedSymbols = append(confirmedSymbols, sym)
			if len(confirmedSymbols) >= e.config.MaxSymbols {
				break
			}
		}
	}
	for _, pkg := range pkgRefs {
		nodes, err := e.index.QuerySymbol(pkg)
		if err == nil && len(nodes) > 0 {
			anchors.PackageRefs = append(anchors.PackageRefs, pkg)
		}
	}
	anchors.SymbolNames = confirmedSymbols
	anchors.Raw = input.Query
	return anchors
}

func extractAtMentions(query string) []string {
	re := regexp.MustCompile(`@([\w./-]+)`)
	matches := re.FindAllStringSubmatch(query, -1)
	var paths []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		clean := filepath.Clean(m[1])
		paths = append(paths, clean)
	}
	return paths
}

func extractCamelCase(query string) []string {
	// Simple regex for CamelCase identifiers (starting with uppercase)
	re := regexp.MustCompile(`[A-Z][a-zA-Z0-9]{2,}`)
	matches := re.FindAllString(query, -1)
	unique := make(map[string]struct{})
	var symbols []string
	for _, m := range matches {
		if _, seen := unique[m]; !seen {
			unique[m] = struct{}{}
			symbols = append(symbols, m)
		}
	}
	return symbols
}

func extractPackagePaths(query string) []string {
	// Match patterns like "framework/capability" or "github.com/user/repo"
	re := regexp.MustCompile(`[a-z0-9]+(/[a-z0-9\-_\.]+)+`)
	matches := re.FindAllString(query, -1)
	unique := make(map[string]struct{})
	var pkgs []string
	for _, m := range matches {
		if _, seen := unique[m]; !seen {
			unique[m] = struct{}{}
			pkgs = append(pkgs, m)
		}
	}
	return pkgs
}
