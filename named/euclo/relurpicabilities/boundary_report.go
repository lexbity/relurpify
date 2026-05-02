package relurpicabilities

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	frameworkast "codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type BoundaryReportHandler struct {
	env agentenv.WorkspaceEnvironment
}

func NewBoundaryReportHandler(env agentenv.WorkspaceEnvironment) *BoundaryReportHandler {
	return &BoundaryReportHandler{env: env}
}

func (h *BoundaryReportHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:cap.boundary_report",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "Boundary Report",
		Version:       "1.0.0",
		Description:   "Generates a workspace layering report with dependency counts and violations",
		Category:      "architecture",
		Tags:          []string{"architecture", "imports", "report"},
		Source:        core.CapabilitySource{Scope: core.CapabilityScopeBuiltin},
		TrustClass:    core.TrustClassBuiltinTrusted,
		RiskClasses:   []core.RiskClass{core.RiskClassReadOnly},
		EffectClasses: []core.EffectClass{},
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"layer": {Type: "string"},
			},
		},
		OutputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"success":           {Type: "boolean"},
				"report":            {Type: "string"},
				"summary":           {Type: "string"},
				"violations":        {Type: "array", Items: &core.Schema{Type: "object"}},
				"dependency_counts": {Type: "object"},
			},
		},
	}
}

func (h *BoundaryReportHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	if h.env.IndexManager == nil {
		return failResult("IndexManager not available in environment"), fmt.Errorf("index manager not available")
	}
	layer, _ := stringArg(args, "layer")
	if strings.TrimSpace(layer) == "" {
		layer = "all"
	}
	store := h.env.IndexManager.Store()
	if store == nil {
		return failResult("index store not available"), fmt.Errorf("index store not available")
	}
	edges, err := store.GetEdgesByType(frameworkast.EdgeTypeImports)
	if err != nil {
		return failResult(fmt.Sprintf("failed to load import edges: %v", err)), err
	}

	workspace := workspaceRoot(h.env)
	dependencyCounts := make(map[string]int)
	violations := make([]interface{}, 0)
	checked := 0

	for _, edge := range edges {
		if edge == nil {
			continue
		}
		sourceNode, _ := store.GetNode(edge.SourceID)
		targetNode, _ := store.GetNode(edge.TargetID)
		importerPath := nodeSourcePath(h.env, sourceNode)
		importeePath := nodeSourcePath(h.env, targetNode)
		if importerPath == "" || importeePath == "" {
			continue
		}
		importerLayer := packageLayerForPath(workspace, importerPath)
		importeeLayer := packageLayerForPath(workspace, importeePath)
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

	return &contracts.CapabilityExecutionResult{
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
