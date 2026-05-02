package relurpicabilities

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// stringArg extracts a string argument from args map.
func stringArg(args map[string]interface{}, key string) (string, bool) {
	val, ok := args[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// intArg extracts an integer argument from args map with a default value.
func intArg(args map[string]interface{}, key string, defaultValue int) (int, bool) {
	val, ok := args[key]
	if !ok {
		return defaultValue, false
	}
	switch v := val.(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	default:
		return defaultValue, false
	}
}

// failResult returns a failure result with an error message.
func failResult(message string) *contracts.CapabilityExecutionResult {
	return &contracts.CapabilityExecutionResult{
		Success: false,
		Data: map[string]interface{}{
			"success": false,
			"error":   message,
		},
	}
}

// truncate limits a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// parseFailedTests extracts failed test names from test output.
// It looks for common patterns like "FAIL: TestName" or "--- FAIL: TestName".
func parseFailedTests(stdout, stderr string) []string {
	var failed []string

	// Common patterns for test failures in Go and other test frameworks
	patterns := []string{
		`FAIL:\s+(\S+)`,      // Go test: FAIL: TestName
		`--- FAIL:\s+(\S+)`,  // Go test verbose: --- FAIL: TestName
		`^\s*(\S+)\s+FAILED`, // Generic: TestName FAILED
	}

	combined := stdout + "\n" + stderr
	lines := strings.Split(combined, "\n")

	for _, line := range lines {
		for _, pattern := range patterns {
			re := regexp.MustCompile(pattern)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				testName := matches[1]
				// Avoid duplicates
				duplicate := false
				for _, existing := range failed {
					if existing == testName {
						duplicate = true
						break
					}
				}
				if !duplicate {
					failed = append(failed, testName)
				}
			}
		}
	}

	return failed
}

// filterNodes filters AST nodes by type and language.
func filterNodes(nodes []*ast.Node, types []ast.NodeType, languages []string) []*ast.Node {
	if len(types) == 0 && len(languages) == 0 {
		return nodes
	}

	var filtered []*ast.Node
	for _, node := range nodes {
		// Check type filter
		if len(types) > 0 {
			typeMatch := false
			for _, t := range types {
				if node.Type == t {
					typeMatch = true
					break
				}
			}
			if !typeMatch {
				continue
			}
		}

		// Check language filter
		if len(languages) > 0 {
			langMatch := false
			for _, lang := range languages {
				if node.Language == lang {
					langMatch = true
					break
				}
			}
			if !langMatch {
				continue
			}
		}

		filtered = append(filtered, node)
	}

	return filtered
}

// nodesToMatchEntries converts AST nodes to match entry maps for output.
func nodesToMatchEntries(nodes []*ast.Node) []map[string]interface{} {
	matches := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		entry := map[string]interface{}{
			"id":          node.ID,
			"name":        node.Name,
			"type":        string(node.Type),
			"category":    string(node.Category),
			"language":    node.Language,
			"start_line":  node.StartLine,
			"end_line":    node.EndLine,
			"file_id":     node.FileID,
			"signature":   node.Signature,
			"is_exported": node.IsExported,
		}
		if node.DocString != "" {
			entry["doc_string"] = node.DocString
		}
		matches = append(matches, entry)
	}
	return matches
}

// traceEntries converts call graph nodes to trace entry maps.
func traceEntries(callees, callers []*ast.Node) []map[string]interface{} {
	entries := make([]map[string]interface{}, 0, len(callees)+len(callers))

	for _, node := range callees {
		entry := map[string]interface{}{
			"id":         node.ID,
			"name":       node.Name,
			"type":       string(node.Type),
			"relation":   "callee",
			"start_line": node.StartLine,
			"file_id":    node.FileID,
		}
		entries = append(entries, entry)
	}

	for _, node := range callers {
		entry := map[string]interface{}{
			"id":         node.ID,
			"name":       node.Name,
			"type":       string(node.Type),
			"relation":   "caller",
			"start_line": node.StartLine,
			"file_id":    node.FileID,
		}
		entries = append(entries, entry)
	}

	return entries
}

// writeRetrievalReferences writes retrieval references to the envelope for AST query results.
func writeRetrievalReferences(env *contextdata.Envelope, query string, nodes []*ast.Node) {
	if env == nil || len(nodes) == 0 {
		return
	}

	// Create chunk IDs for the nodes (in a real implementation, these would be actual chunk IDs)
	chunkIDs := make([]contextdata.ChunkID, 0, len(nodes))
	for _, node := range nodes {
		// Use node ID as a chunk ID for now
		chunkIDs = append(chunkIDs, contextdata.ChunkID(node.ID))
	}

	ref := contextdata.RetrievalReference{
		QueryID:     "ast_query_" + query,
		QueryText:   query,
		Scope:       "workspace",
		ChunkIDs:    chunkIDs,
		TotalFound:  len(nodes),
		FilteredOut: 0,
		RetrievedAt: time.Now(),
		Duration:    0, // Would be measured in a real implementation
	}

	env.References.Retrieval = append(env.References.Retrieval, ref)
}

// graphNodeEntry converts an AST node to a graph node entry map.
func graphNodeEntry(node *ast.Node) map[string]interface{} {
	return map[string]interface{}{
		"id":          node.ID,
		"name":        node.Name,
		"type":        string(node.Type),
		"category":    string(node.Category),
		"language":    node.Language,
		"start_line":  node.StartLine,
		"end_line":    node.EndLine,
		"file_id":     node.FileID,
		"signature":   node.Signature,
		"is_exported": node.IsExported,
	}
}

// graphEdgeEntry converts an AST edge to a graph edge entry map.
func graphEdgeEntry(edge *ast.Edge) map[string]interface{} {
	return map[string]interface{}{
		"id":         edge.ID,
		"source_id":  edge.SourceID,
		"target_id":  edge.TargetID,
		"type":       string(edge.Type),
		"attributes": edge.Attributes,
	}
}

// callGraphToNodesEdges converts a call graph and node set to structured nodes and edges.
func callGraphToNodesEdges(callGraph *ast.CallGraph, nodeSet map[string]*ast.Node, depth int) ([]map[string]interface{}, []map[string]interface{}) {
	// Convert nodes to entries
	nodes := make([]map[string]interface{}, 0, len(nodeSet))
	for _, node := range nodeSet {
		nodes = append(nodes, graphNodeEntry(node))
	}

	// Build edges from call graph
	edges := make([]map[string]interface{}, 0)
	edgeID := 0

	// Add callee edges
	if rootCallees, ok := callGraph.Callees[callGraph.Root.ID]; ok {
		for _, callee := range rootCallees {
			edge := &ast.Edge{
				ID:         fmt.Sprintf("edge_%d", edgeID),
				SourceID:   callGraph.Root.ID,
				TargetID:   callee.ID,
				Type:       ast.EdgeTypeCalls,
				Attributes: map[string]interface{}{},
			}
			edges = append(edges, graphEdgeEntry(edge))
			edgeID++
		}
	}

	// Add caller edges
	if rootCallers, ok := callGraph.Callers[callGraph.Root.ID]; ok {
		for _, caller := range rootCallers {
			edge := &ast.Edge{
				ID:         fmt.Sprintf("edge_%d", edgeID),
				SourceID:   caller.ID,
				TargetID:   callGraph.Root.ID,
				Type:       ast.EdgeTypeCalls,
				Attributes: map[string]interface{}{},
			}
			edges = append(edges, graphEdgeEntry(edge))
			edgeID++
		}
	}

	return nodes, edges
}

// parsePorcelainBlame parses git blame --porcelain output into structured entries.
func parsePorcelainBlame(output string) []map[string]interface{} {
	lines := strings.Split(output, "\n")
	var entries []map[string]interface{}
	var currentEntry map[string]interface{}

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Porcelain format lines start with a 40-char hex SHA
		if len(line) >= 40 && isHex(line[:40]) {
			// Start a new entry
			if currentEntry != nil {
				entries = append(entries, currentEntry)
			}
			parts := strings.Fields(line)
			currentEntry = map[string]interface{}{
				"commit":  parts[0],
				"line":    parts[1],
				"author":  "",
				"summary": "",
			}
		} else if currentEntry != nil {
			// Parse entry fields
			if strings.HasPrefix(line, "author ") {
				currentEntry["author"] = strings.TrimPrefix(line, "author ")
			} else if strings.HasPrefix(line, "summary ") {
				currentEntry["summary"] = strings.TrimPrefix(line, "summary ")
			} else if strings.HasPrefix(line, "author-time ") {
				currentEntry["author_time"] = strings.TrimPrefix(line, "author-time ")
			} else if strings.HasPrefix(line, "committer ") {
				currentEntry["committer"] = strings.TrimPrefix(line, "committer ")
			}
		}
	}

	// Add the last entry
	if currentEntry != nil {
		entries = append(entries, currentEntry)
	}

	return entries
}

// isHex checks if a string is hexadecimal.
func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
