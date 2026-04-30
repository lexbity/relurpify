package relurpicabilities

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

type APICompatHandler struct {
	env agentenv.WorkspaceEnvironment
	frameworkPolicyContext
}

func NewAPICompatHandler(env agentenv.WorkspaceEnvironment) *APICompatHandler {
	return &APICompatHandler{env: env}
}

func (h *APICompatHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:cap.api_compat",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "API Compatibility",
		Version:       "1.0.0",
		Description:   "Compares exported signatures between git refs and flags breaking changes",
		Category:      "migration_compat",
		Tags:          []string{"migration", "compatibility", "git", "api"},
		Source:        core.CapabilitySource{Scope: core.CapabilityScopeBuiltin},
		TrustClass:    core.TrustClassBuiltinTrusted,
		RiskClasses:   []core.RiskClass{core.RiskClassReadOnly},
		EffectClasses: []core.EffectClass{},
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"base_ref": {Type: "string"},
				"head_ref": {Type: "string"},
			},
			Required: []string{"base_ref"},
		},
		OutputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"success":    {Type: "boolean"},
				"breaking":   {Type: "array", Items: &core.Schema{Type: "object"}},
				"compatible": {Type: "array", Items: &core.Schema{Type: "object"}},
				"summary":    {Type: "string"},
				"base_ref":   {Type: "string"},
				"head_ref":   {Type: "string"},
			},
		},
	}
}

func (h *APICompatHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
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
	stdout, stderr, err := h.env.CommandRunner.Run(ctx, listReq)
	combined := stdout + stderr
	if err != nil && strings.TrimSpace(combined) == "" {
		return failResult(fmt.Sprintf("failed to list changed files: %v", err)), err
	}

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

	return &core.CapabilityExecutionResult{
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
	stdout, stderr, err := h.env.CommandRunner.Run(ctx, req)
	if err != nil && strings.TrimSpace(stdout+stderr) == "" {
		return nil, err
	}
	if strings.TrimSpace(stdout) == "" {
		return nil, nil
	}
	return []byte(stdout), nil
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
