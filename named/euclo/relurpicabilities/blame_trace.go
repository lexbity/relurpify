package relurpicabilities

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	"codeburg.org/lexbit/relurpify/governance/taxonomy"
)

// BlameTraceHandler implements the git blame capability.
type BlameTraceHandler struct {
	env agentenv.AgentContext
	frameworkPolicyContext
}

// NewBlameTraceHandler creates a new blame trace handler.
func NewBlameTraceHandler(env agentenv.AgentContext) *BlameTraceHandler {
	return &BlameTraceHandler{env: env}
}

// Descriptor returns the capability descriptor for the blame trace handler.
func (h *BlameTraceHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.blame_trace",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          "Blame Trace",
		Version:       "1.0.0",
		Description:   "Parses git blame output to determine commit and author information for code lines",
		Category:      "git",
		Tags:          []string{"git", "blame", "read-only"},
		Source: descriptor.CapabilitySource{
			Scope: taxonomy.CapabilityScopeBuiltin,
		},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		RiskClasses:   []taxonomy.RiskClass{taxonomy.RiskClassReadOnly},
		EffectClasses: []taxonomy.EffectClass{},
		InputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"file": {
					Type:        "string",
					Description: "File path to blame",
				},
				"lines": {
					Type:        "array",
					Description: "Line range [start, end] to blame (optional)",
					Items: &schemacoerce.Schema{
						Type: "integer",
					},
				},
				"symbol": {
					Type:        "string",
					Description: "Symbol name to resolve to line range (optional, uses IndexManager)",
				},
			},
			Required: []string{"file"},
		},
		OutputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"success": {
					Type:        "boolean",
					Description: "True if blame executed successfully",
				},
				"file": {
					Type:        "string",
					Description: "The blamed file path",
				},
				"entries": {
					Type:        "array",
					Description: "Blame entries per line",
					Items: &schemacoerce.Schema{
						Type: "object",
					},
				},
			},
		},
	}
}

// Invoke executes git blame and returns parsed blame entries.
func (h *BlameTraceHandler) Invoke(ctx context.Context, env ports.State, args map[string]interface{}) (*ports.ToolResult, error) {
	// Extract arguments
	file, ok := stringArg(args, "file")
	if !ok || file == "" {
		return failResult("file argument is required and must be non-empty"), nil
	}

	// Check for CommandRunner
	if h.env.CommandRunner == nil {
		return failResult("CommandRunner not available in environment"), nil
	}

	resolvedFile, err := h.resolveWorkspacePath(h.env, file)
	if err != nil {
		return failResult(fmt.Sprintf("file resolution failed: %v", err)), err
	}

	// Determine line range
	var lineRange string
	if lines, ok := args["lines"].([]interface{}); ok && len(lines) == 2 {
		start, _ := intArg(args, "lines", 0)
		end := 0
		if len(lines) > 1 {
			if endVal, ok := lines[1].(int); ok {
				end = endVal
			} else if endVal, ok := lines[1].(float64); ok {
				end = int(endVal)
			}
		}
		if start > 0 && end > 0 {
			lineRange = fmt.Sprintf("-L%d,%d", start, end)
		}
	}

	// If symbol is provided, resolve to line range using IndexManager
	if symbol, ok := stringArg(args, "symbol"); ok && symbol != "" {
		if h.env.IndexManager != nil {
			nodes, err := h.env.IndexManager.QuerySymbol(symbol)
			if err == nil && len(nodes) > 0 {
				node := nodes[0]
				if node.StartLine > 0 && node.EndLine > 0 {
					lineRange = fmt.Sprintf("-L%d,%d", node.StartLine, node.EndLine)
				}
			}
		}
	}

	// Build git blame command
	cmdArgs := []string{"git", "blame", "--porcelain"}
	if lineRange != "" {
		cmdArgs = append(cmdArgs, lineRange)
	}
	cmdArgs = append(cmdArgs, file)

	req := sandbox.CommandRequest{
		Args:    cmdArgs,
		Workdir: h.env.IndexManager.WorkspacePath(),
	}
	if err := h.authorizeCommand(ctx, h.env, req, "euclo blame trace"); err != nil {
		return failResult(fmt.Sprintf("blame command denied: %v", err)), err
	}
	req.Args[len(req.Args)-1] = resolvedFile

	// Execute command
	res, err := h.env.CommandRunner.Run(ctx, req)
	if err != nil {
		return &ports.ToolResult{
			Success: false,
			Data: map[string]interface{}{
				"success": false,
				"error":   err.Error(),
				"stderr":  "",
			},
		}, nil
	}
	if res.ExitCode != 0 {
		return &ports.ToolResult{
			Success: false,
			Data: map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("exit code %d: %s", res.ExitCode, res.Stderr),
				"stderr":  truncate(res.Stderr, 10000),
			},
		}, nil
	}

	// Parse porcelain blame output
	entries := parsePorcelainBlame(res.Stdout)

	return &ports.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"success": true,
			"file":    resolvedFile,
			"entries": entries,
		},
	}, nil
}
