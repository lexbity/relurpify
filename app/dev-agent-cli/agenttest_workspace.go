package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

var openPreparedRunWorkspaceFn = openPreparedRunWorkspace
var preparedRunRegistrationFuncsFn = euclo.GetRegistrationFuncs

type preparedRunWorkspaceTarget struct {
	Descriptor *agenttest.PreparedRunDescriptor
	Config     agentenv.WorkspaceConfig
}

func buildPreparedRunWorkspaceTarget(desc *agenttest.PreparedRunDescriptor, outputRoot string, opts preparedRunOverrides) (*preparedRunWorkspaceTarget, error) {
	if desc == nil {
		return nil, fmt.Errorf("descriptor required")
	}
	working := *desc
	applyPreparedRunOverrides(&working, opts)
	if err := working.Normalize(); err != nil {
		return nil, err
	}
	setupWorkspace := strings.TrimSpace(working.DerivedWorkspaceRoot)
	if setupWorkspace == "" {
		return nil, fmt.Errorf("descriptor missing derived_workspace_root")
	}
	runRoot := strings.TrimSpace(outputRoot)
	if runRoot == "" {
		runRoot = strings.TrimSpace(working.RunRoot)
	}
	runRoot = preparedRunAbsPath(runRoot)
	if runRoot == "" {
		return nil, fmt.Errorf("run root required")
	}
	executionWorkspace := filepath.Join(runRoot, "execution", "workspace")
	executionLogPath := filepath.Join(runRoot, "execution", "logs", "agenttest.log")
	executionTelemetryPath := filepath.Join(runRoot, "execution", "telemetry", "agenttest.jsonl")
	configPath := workspacePathForExecution(executionWorkspace, setupWorkspace, working.ConfigPath, filepath.Join(executionWorkspace, manifest.DirName, "config.yaml"))
	if configPath == "" {
		return nil, fmt.Errorf("descriptor missing config_path")
	}
	manifestPath := workspacePathForExecution(executionWorkspace, setupWorkspace, working.ManifestPath, filepath.Join(executionWorkspace, manifest.DirName, "agent.manifest.yaml"))
	if manifestPath == "" {
		return nil, fmt.Errorf("descriptor missing manifest_path")
	}
	agentsDir := workspacePathForExecution(executionWorkspace, setupWorkspace, working.AgentsDir, filepath.Join(executionWorkspace, manifest.DirName, "agents"))
	if agentsDir == "" {
		return nil, fmt.Errorf("descriptor missing agents_dir")
	}
	snapshot, err := manifest.LoadAgentManifestSnapshot(working.ManifestPath)
	if err != nil {
		return nil, err
	}
	effectiveContract, err := manifest.ResolveEffectiveAgentContract(executionWorkspace, snapshot.Manifest, manifest.ResolveOptions{}, nil)
	if err != nil {
		return nil, err
	}
	resolvedSpec := *effectiveContract.AgentSpec
	resolvedSpec.Bash.Default = agentspec.AgentPermissionAllow
	working.RunRoot = runRoot
	working.ExecutionDir = filepath.Join(runRoot, "execution")
	working.ExecutionLogsDir = filepath.Join(runRoot, "execution", "logs")
	working.ExecutionTelemetryDir = filepath.Join(runRoot, "execution", "telemetry")
	working.ExecutionArtifactsDir = filepath.Join(runRoot, "execution", "artifacts")
	working.VerificationDir = filepath.Join(runRoot, "verification")
	cfg := agentenv.WorkspaceConfig{
		Workspace:         executionWorkspace,
		ManifestPath:      manifestPath,
		ConfigPath:        configPath,
		InferenceProvider: working.BackendProvider,
		InferenceEndpoint: working.BackendEndpoint,
		InferenceModel:    working.ModelName,
		AgentName:         working.AgentName,
		AgentsDir:         agentsDir,
		SandboxBackend:    working.SandboxBackend,
		LogPath:           executionLogPath,
		TelemetryPath:     executionTelemetryPath,
		SkipASTIndex:      working.SkipASTIndex,
		MaxIterations:     working.MaxIterations,
		AgentSpec:         &resolvedSpec,
	}
	return &preparedRunWorkspaceTarget{Descriptor: &working, Config: cfg}, nil
}

func openPreparedRunWorkspace(ctx context.Context, desc *agenttest.PreparedRunDescriptor, outputRoot string, opts preparedRunOverrides) (*agentenv.Workspace, *preparedRunWorkspaceTarget, error) {
	target, err := buildPreparedRunWorkspaceTarget(desc, outputRoot, opts)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(outputRoot) != "" && preparedRunAbsPath(outputRoot) != preparedRunAbsPath(target.Descriptor.DerivedWorkspaceRoot) {
		if err := os.RemoveAll(target.Config.Workspace); err != nil {
			return nil, nil, err
		}
		if err := agenttest.CopyWorkspace(target.Descriptor.DerivedWorkspaceRoot, target.Config.Workspace, nil); err != nil {
			return nil, nil, err
		}
	}
	ws, err := agentenv.Open(ctx, target.Config, llm.ProviderSecrets{}, preparedRunRegistrationFuncsFn())
	if err != nil {
		return nil, nil, err
	}
	return ws, target, nil
}

func workspacePathForExecution(executionWorkspace, sourceWorkspace, sourcePath, fallback string) string {
	executionWorkspace = strings.TrimSpace(executionWorkspace)
	sourceWorkspace = strings.TrimSpace(sourceWorkspace)
	sourcePath = strings.TrimSpace(sourcePath)
	if executionWorkspace == "" {
		return ""
	}
	if sourceWorkspace != "" && sourcePath != "" {
		if rel, err := filepath.Rel(sourceWorkspace, sourcePath); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join(executionWorkspace, rel)
		}
	}
	if fallback != "" {
		return preparedRunAbsPath(fallback)
	}
	return ""
}

func preparedRunAbsPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}
