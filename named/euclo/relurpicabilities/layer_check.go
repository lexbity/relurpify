package relurpicabilities

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/governance/classification"
)

// LayerCheckHandler implements the import boundary checker capability.
type LayerCheckHandler struct {
	deps IndexDeps
}

// NewLayerCheckHandler creates a new layer check handler.
func NewLayerCheckHandler(deps IndexDeps) *LayerCheckHandler {
	return &LayerCheckHandler{deps: deps}
}

// Descriptor returns the capability descriptor for the layer check handler.
func (h *LayerCheckHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.layer_check",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          "Layer Check",
		Version:       "1.0.0",
		Description:   "Checks import boundaries between architectural layers for violations",
		Category:      "architecture",
		Tags:          []string{"architecture", "imports", "read-only"},
		Source: descriptor.CapabilitySource{
			Scope: classification.CapabilityScopeBuiltin,
		},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		EffectClasses: []classification.EffectClass{},
		InputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"layer": {
					Type:        "string",
					Description: `Layer to check: "framework" | "agents" | "named" | "app" | "all" (default: "all")`,
				},
				"strict": {
					Type:        "boolean",
					Description: "Fail on any violation vs. warn (default: true)",
				},
			},
		},
		OutputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"success": {
					Type:        "boolean",
					Description: "True if check completed",
				},
				"passed": {
					Type:        "boolean",
					Description: "True if no violations found",
				},
				"violations": {
					Type:        "array",
					Description: "Import boundary violations",
					Items:       &schemacoerce.Schema{Type: "object"},
				},
				"layer": {
					Type:        "string",
					Description: "The layer filter applied",
				},
				"checked": {
					Type:        "integer",
					Description: "Number of import edges checked",
				},
			},
		},
	}
}

// layerRule defines a forbidden import relationship between package path prefixes.
type layerRule struct {
	name     string
	importer string // package path prefix of the importing package
	importee string // package path prefix that must not be imported
	layer    string // which layer filter applies this rule
}

var layerRules = []layerRule{
	{"framework must not import agents", "framework/", "agents/", "framework"},
	{"framework must not import named", "framework/", "named/", "framework"},
	{"agents must not import named", "agents/", "named/", "agents"},
	{"ayenitd must not import named", "ayenitd/", "named/", "named"},
	{"framework must not import ayenitd", "framework/", "ayenitd/", "framework"},
	{"platform must not import ayenitd", "platform/", "ayenitd/", "framework"},
	{"framework/services must not import ayenitd", "framework/services/", "ayenitd/", "framework/services"},
	{"framework/agentenv must not import ayenitd", "framework/agentenv/", "ayenitd/", "framework/agentenv"},
}

// Invoke scans the import graph and returns any boundary violations.
func (h *LayerCheckHandler) Invoke(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error) {
	if h.deps.Searcher == nil {
		return failResult("index service not available"), nil
	}

	layer, _ := stringArg(args, "layer")
	if layer == "" {
		layer = "all"
	}

	var applicable []layerRule
	for _, rule := range layerRules {
		if layer == "all" || rule.layer == layer {
			applicable = append(applicable, rule)
		}
	}

	allNodes, err := h.deps.Searcher.SearchNodes(ast.NodeQuery{Categories: []ast.Category{ast.CategoryCode}})
	if err != nil {
		return nil, err
	}

	violations := make([]map[string]any, 0)
	seenViolations := make(map[string]struct{})
	checked := 0

	for _, node := range allNodes {
		if !isLayerCandidate(node) {
			continue
		}
		query := strings.TrimSpace(node.Name)
		if query == "" {
			query = strings.TrimSpace(node.FileID)
		}
		if query == "" {
			continue
		}
		if h.deps.Grapher == nil {
			continue
		}
		depGraph, err := h.deps.Grapher.GetDependencyGraph(query)
		if err != nil {
			continue
		}
		importerPath := stripModulePrefix(depGraph.Root.FileID)
		if importerPath == "" {
			importerPath = stripModulePrefix(node.FileID)
		}
		if importerPath == "" {
			continue
		}

		for _, dep := range depGraph.Dependencies {
			if dep == nil {
				continue
			}
			checked++
			importeePkg := stripModulePrefix(dep.FileID)
			if importeePkg == "" {
				continue
			}

			for _, rule := range applicable {
				if strings.HasPrefix(importerPath, rule.importer) && strings.HasPrefix(importeePkg, rule.importee) {
					key := fmt.Sprintf("%s|%s|%s", importerPath, importeePkg, rule.name)
					if _, ok := seenViolations[key]; ok {
						continue
					}
					seenViolations[key] = struct{}{}
					violations = append(violations, map[string]any{
						"importer": importerPath,
						"importee": importeePkg,
						"rule":     rule.name,
						"layer":    rule.layer,
					})
				}
			}
		}
	}

	passed := len(violations) == 0
	return &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"success":    true,
			"passed":     passed,
			"violations": mapsToInterfaces(violations),
			"layer":      layer,
			"checked":    checked,
		},
	}, nil
}

func isLayerCandidate(node *ast.Node) bool {
	if node == nil {
		return false
	}
	switch node.Type {
	case ast.NodeTypePackage, ast.NodeTypeModule, ast.NodeTypeFunction, ast.NodeTypeMethod, ast.NodeTypeStruct, ast.NodeTypeInterface, ast.NodeTypeClass:
		return true
	default:
		return false
	}
}

func mapsToInterfaces(values []map[string]any) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

// stripModulePrefix removes the Go module path prefix, leaving the package sub-path.
func stripModulePrefix(pkg string) string {
	// Remove common module prefixes so rules can match on relative paths like "framework/"
	prefixes := []string{
		"codeburg.org/lexbit/relurpify/",
		"github.com/lexcodex/relurpify/",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(pkg, p) {
			return strings.TrimPrefix(pkg, p)
		}
	}
	return pkg
}
