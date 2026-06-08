package relurpicabilities

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	"codeburg.org/lexbit/relurpify/governance/taxonomy"
)

type APICompatHandler struct {
	env agentenv.AgentContext
	frameworkPolicyContext
}

func NewAPICompatHandler(env agentenv.AgentContext) *APICompatHandler {
	return &APICompatHandler{env: env}
}

func (h *APICompatHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.api_compat",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          "API Compatibility",
		Version:       "1.0.0",
		Description:   "Compares exported signatures between git refs and flags breaking changes",
		Category:      "migration_compat",
		Tags:          []string{"migration", "compatibility", "git", "api"},
		Source:        descriptor.CapabilitySource{Scope: taxonomy.CapabilityScopeBuiltin},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		RiskClasses:   []taxonomy.RiskClass{taxonomy.RiskClassReadOnly},
		EffectClasses: []taxonomy.EffectClass{},
		InputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"base_ref": {Type: "string"},
				"head_ref": {Type: "string"},
			},
			Required: []string{"base_ref"},
		},
		OutputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"success":    {Type: "boolean"},
				"breaking":   {Type: "array", Items: &schemacoerce.Schema{Type: "object"}},
				"compatible": {Type: "array", Items: &schemacoerce.Schema{Type: "object"}},
				"summary":    {Type: "string"},
				"base_ref":   {Type: "string"},
				"head_ref":   {Type: "string"},
			},
		},
	}
}

func (h *APICompatHandler) Invoke(ctx context.Context, env ports.State, args map[string]interface{}) (*ports.ToolResult, error) {
	if h.env.CommandRunner == nil {
		return failResult("CommandRunner not available in environment"), fmt.Errorf("command runner not available")
	}
	baseRef, ok := stringArg(args, "base_ref")
	if !ok || strings.TrimSpace(baseRef) == "" {
		return failResult("base_ref argument is required"), fmt.Errorf("base_ref argument is required")
	}
	headRef, _ := stringArg(args, "head_ref")
	if strings.TrimSpace(headRef) == "" {
		headRef = "HEAD"
	}

	listReq := sandbox.CommandRequest{
		Workdir: workspaceRoot(h.env),
		Args:    []string{"git", "diff", "--name-only", "--diff-filter=ACMRT", baseRef, headRef, "--", "*.go"},
		Timeout: 30 * time.Second,
	}
	if err := h.authorizeCommand(ctx, h.env, listReq, "euclo api compat"); err != nil {
		return failResult(fmt.Sprintf("api compatibility command denied: %v", err)), err
	}
	res, err := h.env.CommandRunner.Run(ctx, listReq)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return failResult(fmt.Sprintf("failed to list changed files: exit code %d: %s", res.ExitCode, res.Stderr)), err
	}

	combined := res.Stdout + res.Stderr
	paths := splitNonEmptyLines(combined)
	sort.Strings(paths)

	baseRecords := make(map[string]apiSignatureRecord)
	headRecords := make(map[string]apiSignatureRecord)
	for _, path := range paths {
		if src, err := h.readGitFile(ctx, baseRef, path); err != nil {
			return failResult(fmt.Sprintf("failed to read %s at %s: %v", path, baseRef, err)), err
		} else if src != nil {
			records, parseErr := collectExportedAPISignatures(path, src)
			if parseErr != nil {
				return failResult(fmt.Sprintf("failed to parse %s at %s: %v", path, baseRef, parseErr)), parseErr
			}
			mergeSignatureRecords(baseRecords, records)
		}
		if src, err := h.readGitFile(ctx, headRef, path); err != nil {
			return failResult(fmt.Sprintf("failed to read %s at %s: %v", path, headRef, err)), err
		} else if src != nil {
			records, parseErr := collectExportedAPISignatures(path, src)
			if parseErr != nil {
				return failResult(fmt.Sprintf("failed to parse %s at %s: %v", path, headRef, parseErr)), parseErr
			}
			mergeSignatureRecords(headRecords, records)
		}
	}

	breaking, compatible := compareAPISignatures(baseRecords, headRecords)
	summary := fmt.Sprintf("%d breaking changes and %d compatible additions across %d Go files", len(breaking), len(compatible), len(paths))

	return &ports.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"success":    true,
			"breaking":   changeRecordSlice(breaking),
			"compatible": changeRecordSlice(compatible),
			"summary":    summary,
			"base_ref":   baseRef,
			"head_ref":   headRef,
		},
	}, nil
}

func (h *APICompatHandler) readGitFile(ctx context.Context, ref, path string) ([]byte, error) {
	req := sandbox.CommandRequest{
		Workdir: workspaceRoot(h.env),
		Args:    []string{"git", "show", fmt.Sprintf("%s:%s", ref, path)},
		Timeout: 30 * time.Second,
	}
	if err := h.authorizeCommand(ctx, h.env, req, "euclo api compat"); err != nil {
		return nil, err
	}
	res, err := h.env.CommandRunner.Run(ctx, req)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("git show %s:%s: exit code %d: %s", ref, path, res.ExitCode, res.Stderr)
	}
	if strings.TrimSpace(res.Stdout) == "" {
		return nil, nil
	}
	return []byte(res.Stdout), nil
}

func mergeSignatureRecords(dst map[string]apiSignatureRecord, src map[string]apiSignatureRecord) {
	for key, record := range src {
		dst[key] = record
	}
}

func splitNonEmptyLines(input string) []string {
	lines := strings.Split(input, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
