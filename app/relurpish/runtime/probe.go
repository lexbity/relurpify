package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/runtime"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func execLookPathImpl(file string) (string, error) {
	return exec.LookPath(file)
}

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

// OllamaReport surfaces the health of the configured Ollama endpoint.
type OllamaReport struct {
	Endpoint      string
	Healthy       bool
	Models        []string
	SelectedModel string
	Error         string
}

// EnvironmentReport aggregates the runtime environment checks.
type EnvironmentReport struct {
	Workspace string
	Sandbox   SandboxReport
	Ollama    OllamaReport
	Manifest  ManifestSummary
	Config    WorkspaceConfig
	Agent     string
	Timestamp time.Time
}

// StatusSnapshot enriches the environment report with live runtime details.
type StatusSnapshot struct {
	Environment  EnvironmentReport
	PendingHITL  []*runtime.PermissionRequest
	ServerActive bool
	Context      *core.ContextSnapshot
}

// ProbeEnvironment inspects sandbox binaries, Ollama availability, and the
// active manifest for status/reporting surfaces.
func ProbeEnvironment(ctx context.Context, cfg Config) EnvironmentReport {
	sandbox := detectSandbox(ctx, cfg)
	ollama := detectOllama(ctx, cfg)
	manifest := summarizeManifest(cfg.ManifestPath)
	var workspaceCfg WorkspaceConfig
	if wcfg, err := LoadWorkspaceConfig(cfg.ConfigPath); err == nil {
		workspaceCfg = wcfg
	}
	return EnvironmentReport{
		Workspace: cfg.Workspace,
		Sandbox:   sandbox,
		Ollama:    ollama,
		Manifest:  manifest,
		Config:    workspaceCfg,
		Agent:     cfg.AgentLabel(),
		Timestamp: time.Now(),
	}
}

// detectSandbox inspects runsc/docker/containerd availability and versions.
func detectSandbox(ctx context.Context, cfg Config) SandboxReport {
	report := SandboxReport{
		Runsc:      inspectRunsc(ctx, cfg.Sandbox.RunscPath),
		Docker:     inspectDocker(ctx),
		Containerd: inspectContainerd(ctx),
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
		report.Errors = append(report.Errors, "gVisor runtime not fully verified")
	}
	return report
}

// detectOllama queries the Ollama tags endpoint to confirm health + models.
func detectOllama(ctx context.Context, cfg Config) OllamaReport {
	report := OllamaReport{Endpoint: cfg.OllamaEndpoint, SelectedModel: cfg.OllamaModel}
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(cfg.OllamaEndpoint, "/")+"/api/tags", nil)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	resp, err := client.Do(req)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		report.Error = fmt.Sprintf("ollama responded with %s", resp.Status)
		return report
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		report.Error = err.Error()
		return report
	}
	for _, model := range payload.Models {
		report.Models = append(report.Models, model.Name)
		if model.Name == cfg.OllamaModel {
			report.SelectedModel = model.Name
		}
	}
	report.Healthy = true
	return report
}

// runCommand executes a short-lived command and returns stdout or a formatted
// error that includes stderr output.
func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
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
func inspectRunsc(ctx context.Context, binary string) SandboxBinary {
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
	output, err := runCommand(ctx, path, "--version")
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
func inspectDocker(ctx context.Context) SandboxBinary {
	res := SandboxBinary{Name: "docker"}
	path, err := exec.LookPath("docker")
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Path = path
	if version, err := runCommand(ctx, "docker", "--version"); err == nil {
		res.Version = strings.TrimSpace(version)
	}
	runtimesJSON, err := runCommand(ctx, "docker", "info", "--format", "{{json .Runtimes}}")
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
func inspectContainerd(ctx context.Context) SandboxBinary {
	res := SandboxBinary{Name: "containerd"}
	path, err := exec.LookPath("containerd")
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Path = path
	if version, err := runCommand(ctx, "containerd", "--version"); err == nil {
		res.Version = strings.TrimSpace(version)
	}
	configDump, err := runCommand(ctx, "containerd", "config", "dump")
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
	env := ProbeEnvironment(ctx, r.Config)
	snapshot := StatusSnapshot{
		Environment:  env,
		PendingHITL:  r.PendingHITL(),
		ServerActive: r.ServerRunning(),
		Context:      r.Context.Snapshot(),
	}
	return snapshot
}
