package relurpicabilities

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	"codeburg.org/lexbit/relurpify/governance/classification"
)

// BlameTraceHandler implements the git blame capability.
type BlameTraceHandler struct {
	cmd      CommandDeps
	resolver symbolResolver
}

// symbolResolver resolves a symbol name to a line range. nil means unavailable.
type symbolResolver interface {
	QuerySymbol(name string) ([]symbolQueryResult, error)
}

// symbolQueryResult captures the line-range fields blame_trace needs.
type symbolQueryResult struct {
	StartLine int
	EndLine   int
}

// NewBlameTraceHandler creates a new blame trace handler.
func NewBlameTraceHandler(cmd CommandDeps, resolver symbolResolver) *BlameTraceHandler {
	return &BlameTraceHandler{cmd: cmd, resolver: resolver}
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
			Scope: classification.CapabilityScopeBuiltin,
		},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		EffectClasses: []classification.EffectClass{},
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
func (h *BlameTraceHandler) Invoke(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error) {
	file, ok := stringArg(args, "file")
	if !ok || file == "" {
		return failResult("file argument is required and must be non-empty"), nil
	}

	resolvedFile := resolveCandidatePath(file, h.cmd.Workspace)
	if resolvedFile == "" {
		return failResult(fmt.Sprintf("file resolution failed: %s", file)), nil
	}

	var lineRange string
	if lines, ok := args["lines"].([]any); ok && len(lines) == 2 {
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

	if symbol, ok := stringArg(args, "symbol"); ok && symbol != "" {
		if h.resolver != nil {
			nodes, err := h.resolver.QuerySymbol(symbol)
			if err == nil && len(nodes) > 0 {
				first := nodes[0]
				if first.StartLine > 0 && first.EndLine > 0 {
					lineRange = fmt.Sprintf("-L%d,%d", first.StartLine, first.EndLine)
				}
			}
		}
	}

	cmdArgs := []string{"git", "blame", "--porcelain"}
	if lineRange != "" {
		cmdArgs = append(cmdArgs, lineRange)
	}
	cmdArgs = append(cmdArgs, file)

	req := sandbox.CommandRequest{
		Args:    cmdArgs,
		Workdir: h.cmd.Workspace,
	}
	if h.cmd.Policy != nil {
		if err := h.cmd.Policy.AllowCommand(ctx, req); err != nil {
			return failResult(fmt.Sprintf("blame command denied: %v", err)), err
		}
	}
	req.Args[len(req.Args)-1] = resolvedFile

	if h.cmd.Runner == nil {
		return failResult("CommandRunner not available in environment"), nil
	}

	res, err := h.cmd.Runner.Run(ctx, req)
	if err != nil {
		return &ports.ToolResult{
			Success: false,
			Data: map[string]any{
				"success": false,
				"error":   err.Error(),
				"stderr":  "",
			},
		}, nil
	}
	if res.ExitCode != 0 {
		return &ports.ToolResult{
			Success: false,
			Data: map[string]any{
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
		Data: map[string]any{
			"success": true,
			"file":    resolvedFile,
			"entries": entries,
		},
	}, nil
}
