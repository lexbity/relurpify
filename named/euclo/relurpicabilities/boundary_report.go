package relurpicabilities

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	frameworkast "codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/capability/classification"
)

type BoundaryReportHandler struct {
	deps IndexDeps
}

func NewBoundaryReportHandler(deps IndexDeps) *BoundaryReportHandler {
	return &BoundaryReportHandler{deps: deps}
}

func (h *BoundaryReportHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.boundary_report",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          "Boundary Report",
		Version:       "1.0.0",
		Description:   "Generates a workspace layering report with dependency counts and violations",
		Category:      "architecture",
		Tags:          []string{"architecture", "imports", "report"},
		Source:        descriptor.CapabilitySource{Scope: classification.CapabilityScopeBuiltin},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		EffectClasses: []classification.EffectClass{},
		InputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"layer": {Type: "string"},
			},
		},
		OutputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"success":           {Type: "boolean"},
				"report":            {Type: "string"},
				"summary":           {Type: "string"},
				"violations":        {Type: "array", Items: &schemacoerce.Schema{Type: "object"}},
				"dependency_counts": {Type: "object"},
			},
		},
	}
}

func (h *BoundaryReportHandler) Invoke(ctx context.Context, env ports.State, args map[string]interface{}) (*ports.ToolResult, error) {
	if h.deps.Store == nil {
		return failResult("index service not available"), fmt.Errorf("index service not available")
	}
	layer, _ := stringArg(args, "layer")
	if strings.TrimSpace(layer) == "" {
		layer = "all"
	}
	edges, err := h.deps.Store.GetEdgesByType(frameworkast.EdgeTypeImports)
	if err != nil {
		return failResult(fmt.Sprintf("failed to load import edges: %v", err)), err
	}

	dependencyCounts := make(map[string]int)
	violations := make([]interface{}, 0)
	checked := 0

	for _, edge := range edges {
		if edge == nil {
			continue
		}
		sourceNode, _ := h.deps.Store.GetNode(edge.SourceID)
		targetNode, _ := h.deps.Store.GetNode(edge.TargetID)
		importerPath := nodePathFromStore(h.deps.Store, sourceNode, h.deps.Workspace)
		importeePath := nodePathFromStore(h.deps.Store, targetNode, h.deps.Workspace)
		if importerPath == "" || importeePath == "" {
			continue
		}
		importerLayer := packageLayerForPath(h.deps.Workspace, importerPath)
		importeeLayer := packageLayerForPath(h.deps.Workspace, importeePath)
		if importerLayer == "" || importeeLayer == "" {
			continue
		}
		checked++
		dependencyCounts[importerLayer+"->"+importeeLayer]++
		if !boundaryLayerAllowed(layer, importerLayer, importeeLayer) {
			line := 0
			if sourceNode != nil {
				line = sourceNode.StartLine
			}
			violations = append(violations, map[string]interface{}{
				"importer": importerPath,
				"importee": importeePath,
				"rule":     boundaryRuleName(importerLayer, importeeLayer),
				"file":     importerPath,
				"line":     line,
			})
		}
	}

	report := buildBoundaryReportMarkdown(layer, checked, dependencyCounts, violations)
	summary := fmt.Sprintf("%d import edges checked, %d violations found", checked, len(violations))

	return &ports.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"success":           true,
			"passed":            len(violations) == 0,
			"layer":             layer,
			"checked":           checked,
			"violations":        violations,
			"dependency_counts": dependencyCounts,
			"summary":           summary,
			"report":            report,
		},
	}, nil
}

func boundaryLayerAllowed(filter, importerLayer, importeeLayer string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	for _, rule := range layerRules {
		if filter != "all" && rule.layer != filter {
			continue
		}
		if importerLayer == strings.TrimSuffix(rule.importer, "/") && importeeLayer == strings.TrimSuffix(rule.importee, "/") {
			return false
		}
	}
	return true
}

func boundaryRuleName(importerLayer, importeeLayer string) string {
	for _, rule := range layerRules {
		if importerLayer == strings.TrimSuffix(rule.importer, "/") && importeeLayer == strings.TrimSuffix(rule.importee, "/") {
			return rule.name
		}
	}
	return "unknown boundary rule"
}

func buildBoundaryReportMarkdown(layer string, checked int, counts map[string]int, violations []interface{}) string {
	var b strings.Builder
	b.WriteString("# Boundary Report\n\n")
	b.WriteString(fmt.Sprintf("- Layer filter: `%s`\n", layer))
	b.WriteString(fmt.Sprintf("- Import edges checked: %d\n", checked))
	b.WriteString(fmt.Sprintf("- Violations: %d\n\n", len(violations)))
	b.WriteString("## Dependency Counts\n\n")
	if len(counts) == 0 {
		b.WriteString("No import edges matched the selected filter.\n\n")
	} else {
		keys := make([]string, 0, len(counts))
		for key := range counts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		b.WriteString("| Importer -> Importee | Count |\n")
		b.WriteString("| --- | ---: |\n")
		for _, key := range keys {
			b.WriteString(fmt.Sprintf("| `%s` | %d |\n", key, counts[key]))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Violations\n\n")
	if len(violations) == 0 {
		b.WriteString("No layering violations found.\n")
		return b.String()
	}
	b.WriteString("| Importer | Importee | Rule | File | Line |\n")
	b.WriteString("| --- | --- | --- | --- | ---: |\n")
	for _, raw := range violations {
		entry, _ := raw.(map[string]interface{})
		b.WriteString(fmt.Sprintf("| `%v` | `%v` | %v | `%v` | %v |\n",
			entry["importer"], entry["importee"], entry["rule"], entry["file"], entry["line"]))
	}
	return b.String()
}
