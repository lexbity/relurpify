package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

func execLookPathImpl(file string) (string, error) {
	return exec.LookPath(file)
}

var newManagedBackend = llm.New

type SandboxBinary struct {
	Name          string
	Path          string
	Version       string
	Error         string
	SupportsRunsc bool
}

type SandboxReport struct {
	Runsc      SandboxBinary
	Docker     SandboxBinary
	Containerd SandboxBinary
	Errors     []string
	Verified   bool
}

// ManifestSummary describes the manifest currently selected by relurpify_cfg.
type ManifestSummary struct {
	Path        string
	Exists      bool
	AgentName   string
	Runtime     string
	Permissions int
	Network     int
	Error       string
	UpdatedAt   time.Time
}

// InferenceBackendReport surfaces the health of the configured inference backend.
type InferenceBackendReport struct {
	Provider        string
	Endpoint        string
	State           llm.BackendHealthState
	Models          []string
	SelectedModel   string
	SelectedProfile string
	ProfileReason   string
	ProfileSource   string
	Error           string
	Resources       *llm.ResourceSnapshot
}

// EnvironmentReport aggregates the runtime environment checks.
type EnvironmentReport struct {
	Workspace string
	Sandbox   SandboxReport
	Inference InferenceBackendReport
	Manifest  ManifestSummary
	Config    WorkspaceConfig
	Agent     string
	Timestamp time.Time
}

// StatusSnapshot enriches the environment report with live runtime details.
type StatusSnapshot struct {
	Environment           EnvironmentReport
	PendingHITL           []*authorization.PermissionRequest
	ServerActive          bool
	ProtectedPaths        []string
	ManifestFingerprint   string
	ManifestPolicySummary string
	SelectedProfile       string
	ProfileReason         string
	ProfileSource         string
	DeprecationNotices    []string
}

// ProbeEnvironment inspects sandbox binaries, inference backend availability,
// and the active manifest for status/reporting surfaces.
func ProbeEnvironment(ctx context.Context, cfg Config, backend llm.ManagedBackend) EnvironmentReport {
	sandbox := detectSandbox(ctx, cfg)
	inference := detectInferenceBackend(ctx, cfg, backend)
	manifest := summarizeManifest(cfg.ManifestPath)
	var workspaceCfg WorkspaceConfig
	if wcfg, err := LoadWorkspaceConfig(cfg.ConfigPath); err == nil {
		workspaceCfg = wcfg
	}
	return EnvironmentReport{
		Workspace: cfg.Workspace,
		Sandbox:   sandbox,
		Inference: inference,
		Manifest:  manifest,
		Config:    workspaceCfg,
		Agent:     cfg.AgentLabel(),
		Timestamp: time.Now(),
	}
}

// detectSandbox inspects runsc/docker/containerd availability and versions.
func detectSandbox(ctx context.Context, cfg Config) SandboxReport {
	report := SandboxReport{
		Runsc:      inspectRunsc(ctx, cfg.Sandbox.RunscPath, cfg.CommandPolicy),
		Docker:     inspectDocker(ctx, cfg.CommandPolicy),
		Containerd: inspectContainerd(ctx, cfg.CommandPolicy),
	}
	if report.Runsc.Error != "" {
		report.Errors = append(report.Errors, report.Runsc.Error)
	}
	if report.Docker.Error != "" {
		report.Errors = append(report.Errors, fmt.Sprintf("docker: %s", report.Docker.Error))
	}
	if report.Containerd.Error != "" {
		report.Errors = append(report.Errors, fmt.Sprintf("containerd: %s", report.Containerd.Error))
	}
	report.Verified = report.Runsc.Error == "" && (report.Docker.SupportsRunsc || report.Containerd.SupportsRunsc)
	if !report.Verified {
		report.Errors = append(report.Errors, "sandbox runtime not fully verified")
	}
	return report
}

// detectInferenceBackend queries the managed backend facade for health + models.
func detectInferenceBackend(ctx context.Context, cfg Config, backend llm.ManagedBackend) InferenceBackendReport {
	report := InferenceBackendReport{
		Provider: cfg.InferenceProvider,
		Endpoint: cfg.InferenceEndpoint,
	}
	ownedBackend := false
	if backend == nil {
		var err error
		backend, err = newManagedBackend(llm.ProviderConfigFromRuntimeConfig(cfg))
		if err != nil {
			report.Error = err.Error()
			report.State = llm.BackendHealthUnhealthy
			return report
		}
		ownedBackend = true
	}
	if ownedBackend {
		defer backend.Close()
	}
	health, err := backend.Health(ctx)
	if health != nil {
		report.State = health.State
		report.Resources = health.Resources
		if health.Message != "" && report.Error == "" {
			report.Error = health.Message
		}
	}
	if err != nil {
		if report.Error == "" {
			report.Error = err.Error()
		}
		if report.State == "" {
			report.State = llm.BackendHealthUnhealthy
		}
	} else if report.State == "" {
		report.State = llm.BackendHealthReady
	}
	models, err := backend.ListModels(ctx)
	if err != nil {
		report.Error = err.Error()
		if report.State == "" {
			report.State = llm.BackendHealthUnhealthy
		}
		return report
	}
	for _, model := range models {
		report.Models = append(report.Models, model.Name)
	}
	selected := strings.TrimSpace(cfg.InferenceModel)
	if selected == "" && len(models) > 0 {
		selected = models[0].Name
	}
	report.SelectedModel = selected
	if reg, err := llm.NewProfileRegistry(manifest.New(cfg.Workspace).ModelProfilesDir()); err == nil {
		resolution := reg.Resolve(cfg.InferenceProvider, selected)
		report.SelectedProfile = filepath.Base(resolution.SourcePath)
		if report.SelectedProfile == "." || report.SelectedProfile == "" {
			report.SelectedProfile = resolution.MatchKind
		}
		report.ProfileReason = resolution.Reason
		report.ProfileSource = resolution.SourcePath
	}
	for _, model := range models {
		if model.Name == selected {
			return report
		}
	}
	if selected == "" {
		report.Error = "inference backend returned no models"
		if report.State == "" {
			report.State = llm.BackendHealthUnhealthy
		}
		return report
	}
	report.Error = fmt.Sprintf("model %s not found in inference backend", selected)
	if report.State == "" {
		report.State = llm.BackendHealthDegraded
	}
	return report
}

// runCommand executes a short-lived command and returns stdout or a formatted
// error that includes stderr output.
func runCommand(ctx context.Context, policy sandbox.CommandPolicy, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if policy != nil {
		if err := policy.AllowCommand(cctx, sandbox.CommandRequest{
			Args: append([]string{name}, args...),
		}); err != nil {
			return "", err
		}
	}
	cmd := exec.CommandContext(cctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return "", fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), detail)
		}
		return "", err
	}
	return stdout.String(), nil
}

// inspectRunsc checks for the runsc binary, version, and runsc support flag.
func inspectRunsc(ctx context.Context, binary string, policy sandbox.CommandPolicy) SandboxBinary {
	if binary == "" {
		binary = "runsc"
	}
	res := SandboxBinary{Name: "runsc"}
	path, err := exec.LookPath(binary)
	if err != nil {
		res.Error = fmt.Sprintf("runsc not found: %v", err)
		return res
	}
	res.Path = path
	output, err := runCommand(ctx, policy, path, "--version")
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Version = strings.TrimSpace(output)
	res.SupportsRunsc = strings.Contains(strings.ToLower(res.Version), "runsc")
	return res
}

// inspectDocker ensures docker exists, captures its version, and checks if the
// runsc runtime is registered.
func inspectDocker(ctx context.Context, policy sandbox.CommandPolicy) SandboxBinary {
	res := SandboxBinary{Name: "docker"}
	path, err := exec.LookPath("docker")
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Path = path
	if version, err := runCommand(ctx, policy, "docker", "--version"); err == nil {
		res.Version = strings.TrimSpace(version)
	}
	runtimesJSON, err := runCommand(ctx, policy, "docker", "info", "--format", "{{json .Runtimes}}")
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.SupportsRunsc = dockerSupportsRunsc(runtimesJSON)
	if !res.SupportsRunsc {
		res.Error = "runsc runtime not registered"
	}
	return res
}

// inspectContainerd confirms containerd is installed and configured with runsc.
func inspectContainerd(ctx context.Context, policy sandbox.CommandPolicy) SandboxBinary {
	res := SandboxBinary{Name: "containerd"}
	path, err := exec.LookPath("containerd")
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Path = path
	if version, err := runCommand(ctx, policy, "containerd", "--version"); err == nil {
		res.Version = strings.TrimSpace(version)
	}
	configDump, err := runCommand(ctx, policy, "containerd", "config", "dump")
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.SupportsRunsc = strings.Contains(configDump, "runsc")
	if !res.SupportsRunsc {
		res.Error = "runsc runtime not configured"
	}
	return res
}

// dockerSupportsRunsc parses the docker runtime map looking for runsc entries.
func dockerSupportsRunsc(payload string) bool {
	var runtimes map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &runtimes); err != nil {
		return strings.Contains(payload, "runsc")
	}
	for name := range runtimes {
		if strings.Contains(strings.ToLower(name), "runsc") {
			return true
		}
	}
	return false
}

func summarizeManifest(path string) ManifestSummary {
	summary := ManifestSummary{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			summary.Error = ""
		} else {
			summary.Error = err.Error()
		}
		return summary
	}
	summary.Exists = true
	summary.UpdatedAt = info.ModTime()
	m, err := manifest.LoadAgentManifest(path)
	if err != nil {
		summary.Error = err.Error()
		return summary
	}
	summary.AgentName = m.Metadata.Name
	summary.Runtime = m.Spec.Runtime
	permFS := len(m.Spec.Permissions.FileSystem)
	permExec := len(m.Spec.Permissions.Executables)
	permNet := len(m.Spec.Permissions.Network)
	if m.Spec.Defaults != nil && m.Spec.Defaults.Permissions != nil {
		permFS += len(m.Spec.Defaults.Permissions.FileSystem)
		permExec += len(m.Spec.Defaults.Permissions.Executables)
		permNet += len(m.Spec.Defaults.Permissions.Network)
	}
	summary.Permissions = permFS + permExec
	summary.Network = permNet
	return summary
}

// Status collects runtime + environment data for the status view.
func (r *Runtime) Status(ctx context.Context) StatusSnapshot {
	env := ProbeEnvironment(ctx, r.Config, r.Backend)
	snapshot := StatusSnapshot{
		Environment:  env,
		PendingHITL:  r.PendingHITL(),
		ServerActive: r.ServerRunning(),
	}
	if env.Workspace != "" {
		snapshot.ProtectedPaths = manifest.New(env.Workspace).GovernanceRoots(
			r.Config.ManifestPath,
			r.Config.ConfigPath,
		)
	}
	if r.Registration != nil && r.Registration.ManifestSnapshot != nil {
		snapshot.ManifestFingerprint = fmt.Sprintf("%x", r.Registration.ManifestSnapshot.Fingerprint)
		snapshot.DeprecationNotices = append([]string(nil), r.Registration.ManifestSnapshot.Warnings...)
	}
	if r.ProfileResolution.Profile != nil {
		snapshot.SelectedProfile = r.ProfileResolution.Profile.MatchPattern()
	}
	snapshot.ProfileReason = r.ProfileResolution.Reason
	snapshot.ProfileSource = r.ProfileResolution.SourcePath
	if r.Registration != nil && r.Registration.Manifest != nil {
		snapshot.ManifestPolicySummary = summarizeManifestPolicy(r.Registration.Manifest)
	}
	return snapshot
}
