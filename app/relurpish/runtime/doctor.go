package runtime

import (
	"codeburg.org/lexbit/relurpify/userconfig/config/model"
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/ayenitd"
	capabilityagentspec "codeburg.org/lexbit/relurpify/capability/agentspec"
	capabilitydescriptor "codeburg.org/lexbit/relurpify/capability/descriptor"
	capabilityregistry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/named/euclo/euclocontract"
	eucloservices "codeburg.org/lexbit/relurpify/named/euclo/services"
	thoughtrecipes "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	platformfs "codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
	"codeburg.org/lexbit/relurpify/userconfig/modelselect"
	templatesembed "codeburg.org/lexbit/relurpify/userconfig/templates/embedfs"
)

// DependencyStatus captures one local dependency check.
type DependencyStatus struct {
	Name      string
	Required  bool
	Available bool
	Degraded  bool
	Blocking  bool
	Details   string
}

// ProviderHealth captures the health and metadata of a single catalog provider.
type ProviderHealth struct {
	Name      string
	Kind      string
	Endpoint  string
	Models    []string
	State     string
	SetupHint string
	Selected  bool
	Error     string
}

// DoctorReport summarizes workspace readiness and local dependency state.
type DoctorReport struct {
	Workspace             string
	ConfigRoot            string
	WorkspacePresent      bool
	ConfigExists          bool
	ManifestExists        bool
	ModelProfilesExists   bool
	StarterTemplatesReady bool
	ConfigError           string
	ManifestError         string
	ModelProfilesError    string
	StarterTemplatesError string
	// Recipes reports the canonical thoughtrecipe set state (Q6: missing
	// recipes are reported, not hidden).
	RecipesReady          bool
	RecipesError          string
	RecipesFound          []string
	RecipesMissing        []string
	ManifestWarnings      []string
	DeprecationNotices    []string
	ProtectedPaths        []string
	ContractSource        string // "builtin+split" | "manifest"
	ManifestFingerprint   string
	ManifestPolicySummary string
	Inference             InferenceBackendReport
	Providers             []ProviderHealth
	Dependencies          []DependencyStatus
	CheckedAt             time.Time

	// SandboxReady is true when the sandbox backend is verified,
	// policy is loaded, and a filesystem scope is constructed.
	// When false, tools are hard-denied regardless of model status.
	SandboxReady bool
	// ModelReady is true when the inference backend reports healthy.
	// When false, guest chat is blocked but tools may still run
	// if SandboxReady is true (e.g. tape/offline sessions).
	ModelReady bool
}

func (r DoctorReport) HasBlockingIssues() bool {
	if !r.ConfigExists {
		return true
	}
	if r.ConfigError != "" || r.ManifestError != "" || r.ModelProfilesError != "" || r.StarterTemplatesError != "" {
		return true
	}
	for _, dep := range r.Dependencies {
		if dep.Blocking {
			return true
		}
	}
	return false
}

func (r DoctorReport) NeedsInitialization() bool {
	return !r.WorkspacePresent || !r.ConfigExists
}

// Ready reports whether the workspace is fully operational:
// both sandbox and model are verified, and no blocking issues exist.
func (r DoctorReport) Ready() bool {
	return r.SandboxReady && r.ModelReady && !r.HasBlockingIssues()
}

// BuildDoctorReport checks workspace state and local runtime dependencies
// without requiring the runtime to start successfully.
func BuildDoctorReport(ctx context.Context, cfg Config, secrets config.Secrets) DoctorReport {
	// Diagnostics are non-interactive: strip the HITL-gated command policy so
	// dependency/chromium probes (and ProbeEnvironment below) never block on
	// approval. See diagnosticConfig.
	cfg = diagnosticConfig(cfg)
	paths := config.New(cfg.Workspace)
	report := DoctorReport{
		Workspace:  cfg.Workspace,
		ConfigRoot: paths.ConfigRoot(),
		CheckedAt:  time.Now().UTC(),
	}
	if info, err := os.Stat(paths.ConfigRoot()); err == nil && info.IsDir() {
		report.WorkspacePresent = true
	}
	if _, err := os.Stat(cfg.ConfigPath); err == nil {
		report.ConfigExists = true
		// Try V1 nested format first (relurpify/workspace/v1).
		// If V1 is unavailable, also inspect the flat workspace config.
		v1Cfg, v1Err := config.LoadRuntimeWorkspaceConfigV1(cfg.ConfigPath)
		if v1Err == nil {
			if v1Cfg.Model.Provider != "" && (strings.TrimSpace(cfg.InferenceProvider) == "" || strings.EqualFold(strings.TrimSpace(cfg.InferenceProvider), "ollama")) {
				cfg.InferenceProvider = v1Cfg.Model.Provider
			}
			if v1Cfg.Model.Name != "" {
				report.Inference.SelectedModel = v1Cfg.Model.Name
			}
			if v1Cfg.Sandbox.Backend != "" && cfg.SandboxBackend == "" {
				cfg.SandboxBackend = v1Cfg.Sandbox.Backend
			}
		}
		// Also inspect the flat loader for fields not present in V1
		// (TapePath, Agents, etc.).
		if loaded, err := config.LoadRuntimeWorkspaceConfig(cfg.ConfigPath); err == nil {
			if loaded.SandboxBackend != "" && cfg.SandboxBackend == "" {
				cfg.SandboxBackend = loaded.SandboxBackend
			}
			if loaded.Provider != "" && (strings.TrimSpace(cfg.InferenceProvider) == "" || strings.EqualFold(strings.TrimSpace(cfg.InferenceProvider), "ollama")) {
				cfg.InferenceProvider = loaded.Provider
			}
			if loaded.Model != "" && cfg.InferenceModel == "" {
				cfg.InferenceModel = loaded.Model
			}
			if loaded.TapePath != "" && cfg.InferenceTapePath == "" {
				cfg.InferenceTapePath = loaded.TapePath
			}
		} else if v1Err != nil {
			// Both loaders failed — surface the V1 error.
			report.ConfigError = v1Err.Error()
		}
	}
	if strings.EqualFold(strings.TrimSpace(cfg.InferenceProvider), "tape") && strings.TrimSpace(cfg.InferenceTapePath) == "" {
		cfg.InferenceTapePath = config.DefaultWorkspaceStateTapeFile(cfg.Workspace)
	}
	report.ContractSource = "builtin+split"
	fp := config.ContractFingerprint(euclocontract.DefaultContract(), cfg.Workspace)
	report.ManifestFingerprint = fmt.Sprintf("%x", fp)
	// Deduplicate sandbox roots (FR-8)
	rawPaths := config.New(cfg.Workspace).GovernanceRoots(config.DefaultWorkspaceConfigPath(cfg.Workspace))
	report.ProtectedPaths = uniqueStrings(rawPaths)

	report.StarterTemplatesReady = checkEmbeddedTemplates()
	if !report.StarterTemplatesReady {
		report.StarterTemplatesError = fmt.Errorf("embedded templates not found").Error()
	}
	recipesCheck := checkCanonicalRecipes(cfg.Workspace)
	report.RecipesReady = recipesCheck.ready
	report.RecipesError = recipesCheck.errText
	report.RecipesFound = recipesCheck.found
	report.RecipesMissing = recipesCheck.missing

	var env EnvironmentReport
	backend, err := llm.New(llm.ProviderConfigFromRuntimeConfig(cfg), llm.ProviderSecrets{
		APIKey: secrets.LLMAPIKey,
	})
	if err != nil {
		env = ProbeEnvironment(ctx, cfg, secrets, nil)
		if env.Inference.Error == "" {
			env.Inference.Error = err.Error()
		}
		env.Inference.State = llm.BackendHealthUnhealthy
	} else {
		defer func() { _ = backend.Close() }()
		env = ProbeEnvironment(ctx, cfg, secrets, backend)
	}
	report.Inference = env.Inference
	// Build provider catalog health list.
	bundle, diags, err := config.LoadDiagnostic(config.LoadOptions{WorkspaceRoot: cfg.Workspace})
	switch {
	case err == nil && bundle.Config != nil:
		reg, _ := buildProviderRegistry(bundle.Config.Model.Providers)
		if reg != nil {
			report.Providers = probeProviderCatalog(ctx, bundle.Config.Model.Providers, cfg, secrets)
		}
		// Model profile check using the same bundle.
		regProfiles := modelselect.NewProfileRegistryFromProfiles(bundle.Config.Model.Profiles)
		resolution := regProfiles.Resolve(cfg.InferenceProvider, report.Inference.SelectedModel)
		profileDiag := firstConfigDiagnostic(diags, "profile")
		switch {
		case resolution.SourcePath != "":
			report.ModelProfilesExists = true
		case profileDiag != nil && strings.EqualFold(strings.TrimSpace(profileDiag.Severity), "blocking"):
			report.ModelProfilesError = profileDiag.Message
		default:
			report.ModelProfilesError = "no workspace model profile matched the selected model"
		}
	case err != nil:
		report.ModelProfilesError = err.Error()
	default:
		report.ModelProfilesError = "workspace config bundle unavailable"
	}
	// Convert ayenitd probe results
	// Map available Config fields to ayenitd.WorkspaceConfig.
	// Some fields may be missing in Config; use zero values.
	ayenitdCfg := ayenitd.WorkspaceConfig{
		Workspace:                  cfg.Workspace,
		InferenceProvider:          cfg.InferenceProvider,
		InferenceEndpoint:          cfg.InferenceEndpoint,
		InferenceModel:             cfg.InferenceModel,
		InferenceTapePath:          cfg.InferenceTapePath,
		InferenceNativeToolCalling: cfg.InferenceNativeToolCalling,
		ConfigPath:                 cfg.ConfigPath,
		AgentsDir:                  cfg.AgentsDir,
		AgentName:                  cfg.AgentName,
		LogPath:                    cfg.LogPath,
		TelemetryPath:              cfg.TelemetryPath,
		EventsPath:                 cfg.EventsPath,
		MemoryPath:                 cfg.MemoryPath,
		HITLTimeout:                cfg.HITLTimeout,
		AuditLimit:                 cfg.AuditLimit,
		SandboxBackend:             cfg.SandboxBackend,
		Sandbox:                    cfg.Sandbox,
	}
	ayenitdResults := ayenitd.ProbeWorkspace(ctx, ayenitdCfg, llm.ProviderSecrets{APIKey: secrets.LLMAPIKey}, nil)
	var deps []DependencyStatus
	for _, r := range ayenitdResults {
		if r.Name == "inference_backend" {
			continue // shown in dedicated Inference backend block (FR-8)
		}
		deps = append(deps, DependencyStatus{
			Name:      r.Name,
			Required:  r.Required,
			Available: r.OK,
			Blocking:  r.Required && !r.OK,
			Details:   r.Message,
		})
	}
	deps = append(deps, DependencyStatus{
		Name:      "starter-templates",
		Required:  true,
		Available: report.StarterTemplatesReady,
		Blocking:  report.StarterTemplatesError != "",
		Details:   firstNonEmpty(report.StarterTemplatesError, "workspace template archive available"),
	})
	deps = append(deps, DependencyStatus{
		Name:      "model-profile",
		Required:  true,
		Available: report.ModelProfilesExists,
		Blocking:  report.ModelProfilesError != "",
		Details:   firstNonEmpty(report.ModelProfilesError, report.Inference.SelectedProfile, "workspace profile available"),
	})
	// Keep existing sandbox and chromium checks
	runscOK := env.Sandbox.Runsc.Error == ""
	dockerOK := env.Sandbox.Docker.Error == ""
	dockerDegraded := env.Sandbox.Docker.Path != "" && !dockerOK
	deps = append(deps, DependencyStatus{
		Name:      "sandbox_backend",
		Required:  true,
		Available: runscOK || dockerOK,
		Blocking:  report.ConfigError == "" && (!runscOK && !dockerOK),
		Details:   formatSandboxDetail(firstNonEmpty(env.Sandbox.Runsc.Version, env.Sandbox.Docker.Version, env.Sandbox.Runsc.Error, env.Sandbox.Docker.Error)),
	})
	deps = append(deps, DependencyStatus{
		Name:      "runsc",
		Required:  false,
		Available: runscOK,
		Blocking:  false,
		Details:   formatSandboxDetail(firstNonEmpty(env.Sandbox.Runsc.Version, env.Sandbox.Runsc.Error)),
	})
	dockerDetail := formatSandboxDetail(firstNonEmpty(env.Sandbox.Docker.Version, env.Sandbox.Docker.Error))
	if dockerDegraded {
		dockerDetail = "degraded: docker installed but unreachable"
	}
	deps = append(deps, DependencyStatus{
		Name:      "docker",
		Required:  false,
		Available: dockerOK,
		Degraded:  dockerDegraded,
		Blocking:  false,
		Details:   dockerDetail,
	})
	// inference_backend is shown in the dedicated "Inference backend:" block,
	// not duplicated here as a dependency entry (FR-8).
	deps = append(deps, detectChromiumStatus(ctx, cfg.CommandPolicy))
	deps = append(deps, probeAuditChain(cfg))
	if bundle.Config != nil {
		deps = append(deps, probeRuntimeImage(bundle.Config.Security.Sandbox))
	}
	report.Dependencies = deps

	report.SandboxReady = computeSandboxReady(report)
	report.ModelReady = computeModelReady(report)
	return report
}

func uniqueStrings(s []string) []string {
	if len(s) < 2 {
		return s
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func computeSandboxReady(r DoctorReport) bool {
	if r.ConfigError != "" || r.StarterTemplatesError != "" {
		return false
	}
	if !r.ConfigExists || !r.StarterTemplatesReady {
		return false
	}
	return true
}

func computeModelReady(r DoctorReport) bool {
	return r.Inference.State == llm.BackendHealthReady || r.Inference.State == llm.BackendHealthDegraded
}

func checkEmbeddedTemplates() bool {
	efs := templatesembed.DefaultFS()
	// Verify the key template files exist in the embed.
	for _, path := range []string{
		"workspace/workspace.yaml",
		"workspace/model/profiles/default.llm.yaml",
		"workspace/model/provider/ollama.provider.yaml",
		"workspace/security/sandbox.policy.yaml",
		"workspace/security/shell.policy.yaml",
		"workspace/security/localtool.policy.yaml",
		"workspace/security/workspaceingestion.policy.yaml",
	} {
		if _, err := efs.Open(path); err != nil {
			return false
		}
	}
	return true
}

// InitializeWorkspaceFromTemplates materializes the full default workspace
// configuration tree under <workspace>/relurpify_cfg from the embedded template
// bundle: workspace.yaml, model profiles + provider catalog, security policies,
// and tool definitions. When overwrite is false, existing files are preserved
// (idempotent re-run).
func InitializeWorkspaceFromTemplates(cfg Config, overwrite bool) error {
	if cfg.Workspace == "" {
		return fmt.Errorf("workspace path required")
	}
	configRoot := config.New(cfg.Workspace).ConfigRoot()
	if err := os.MkdirAll(configRoot, platformfs.PublicDirMode); err != nil { // public: config root
		return err
	}
	// The embedded template bundle is the canonical, distribution-safe source:
	// it is compiled into the binary, so initialization materializes the full
	// default tree (workspace.yaml, model profiles + provider catalog, security
	// policies, and tool definitions) even for an installed binary with no
	// source checkout on disk. The embedded "workspace/" subtree maps 1:1 onto
	// <workspace>/relurpify_cfg/.
	efs := templatesembed.DefaultFS()
	const templateRoot = "workspace"
	walkErr := fs.WalkDir(efs, templateRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(templateRoot, filepath.FromSlash(p))
		if relErr != nil {
			return relErr
		}
		data, readErr := fs.ReadFile(efs, p)
		if readErr != nil {
			return fmt.Errorf("read embedded template %s: %w", p, readErr)
		}
		return copyTemplateContent(data, filepath.Join(configRoot, rel), cfg.Workspace, overwrite)
	})
	if walkErr != nil {
		return fmt.Errorf("materialize workspace templates: %w", walkErr)
	}
	stateDir := config.DefaultWorkspaceStateDir(cfg.Workspace)
	for _, dir := range []string{
		stateDir,
		filepath.Join(stateDir, "logs"),
		filepath.Join(stateDir, "telemetry"),
		filepath.Join(stateDir, "memory"),
		filepath.Join(stateDir, "sessions"),
		filepath.Join(stateDir, "test_run"),
	} {
		if err := os.MkdirAll(dir, platformfs.PublicDirMode); err != nil { // public: runtime state dirs
			return err
		}
	}
	return nil
}

func copyTemplateContent(data []byte, dst, workspace string, overwrite bool) error {
	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
	}
	rendered := strings.ReplaceAll(string(data), "${workspace}", filepath.ToSlash(workspace))
	cleanDst := filepath.Clean(dst)
	if !strings.HasPrefix(cleanDst, filepath.Clean(workspace)) {
		return fmt.Errorf("path traversal: %s", dst)
	}
	if err := os.MkdirAll(filepath.Dir(cleanDst), platformfs.PublicDirMode); err != nil { // public: template dst dir
		return err
	}
	return os.WriteFile(filepath.Clean(cleanDst), []byte(rendered), platformfs.PublicFileMode) //nolint:gosec // workspace-scoped template output after prefix check
}

func detectChromiumStatus(ctx context.Context, policy sandbox.CommandPolicy) DependencyStatus {
	binaries := []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"}
	for _, name := range binaries {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		version, _ := runCommand(ctx, policy, path, "--version")
		return DependencyStatus{
			Name:      "chromium",
			Required:  false,
			Available: true,
			Blocking:  false,
			Details:   strings.TrimSpace(firstNonEmpty(version, path)),
		}
	}
	return DependencyStatus{
		Name:      "chromium",
		Required:  false,
		Available: false,
		Blocking:  false,
		Details:   "not found",
	}
}

func formatSandboxDetail(detail string) string {
	if detail == "" {
		return "sandbox unavailable — agent runtime will FAIL TO START (no host-exec fallback)"
	}
	// If it's an error message, append the note
	if strings.Contains(detail, "error") || strings.Contains(detail, "not found") {
		return detail + " — agent runtime will FAIL TO START (no host-exec fallback)"
	}
	// If it's a version string, we're good
	return detail
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// probeAuditChain reports audit-chain readiness: dir writability and chain
// integrity across every agent chain under <state>/audit. In strict
// enforcement (the default) a broken chain or unwritable state dir is
// BLOCKING — governed actions fail closed without durable audit (SBH-1 D-10).
// In best_effort the same conditions degrade to warnings.
func probeAuditChain(cfg Config) DependencyStatus {
	auditRoot := filepath.Join(config.DefaultWorkspaceStateDir(cfg.Workspace), "audit")
	strict := !strings.EqualFold(strings.TrimSpace(cfg.AuditEnforcement), "best_effort")
	writable := probeWritableDir(auditRoot)

	// Aggregate over every agent chain subdirectory. A single broken chain is
	// a broken chain; the probe result carries the worst finding.
	var probe policy.ChainProbe
	present := false
	broken := false
	entries, err := os.ReadDir(auditRoot)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			p := policy.ProbeChainDir(filepath.Join(auditRoot, e.Name()))
			if p.Present {
				present = true
			}
			if p.Broken {
				broken = true
				probe = p
			} else if !present {
				probe = p
			}
		}
	}

	detail := ""
	switch {
	case !present && !writable:
		detail = "audit dir missing and not writable — governed actions will fail closed"
	case !present:
		detail = "no audit chain yet (created on first agent run)"
	case broken:
		detail = "audit chain integrity FAILED (" + probe.Failure + ")"
		detail = strings.TrimSpace(detail) + " — tamper detected, chain rotated on next boot"
	default:
		detail = fmt.Sprintf("audit chain ok (last sequence %d)", probe.LastSeq)
	}
	return DependencyStatus{
		Name:      "audit_chain",
		Required:  true,
		Available: !broken && writable,
		Degraded:  !strict && (broken || !writable),
		Blocking:  strict && (broken || !writable),
		Details:   detail,
	}
}

// probeWritableDir verifies the directory exists and can host a fresh file.
func probeWritableDir(dir string) bool {
	if dir == "" {
		return false
	}
	if err := os.MkdirAll(dir, platformfs.PublicDirMode); err != nil {
		return false
	}
	probe := filepath.Join(dir, ".doctor-write-probe")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return true
}

// probeRuntimeImage surfaces the sandbox runtime image pin posture (SBH-1
// D-13): a stated image_digest is pinned short-form; its absence reads as
// "unpinned/tag-based with boot-time daemon resolution" and is a warning, not
// a block.
func probeRuntimeImage(sbox *cfgsecurity.SandboxPolicyConfig) DependencyStatus {
	if sbox == nil {
		return DependencyStatus{Name: "runtime_image", Required: true, Available: false, Details: "sandbox policy unavailable"}
	}
	digest := strings.TrimSpace(sbox.ImageDigest)
	detail := "unpinned (tag-based; resolved from the local daemon at boot — set security.sandbox.image_digest to pin)"
	if digest != "" {
		detail = "pinned by digest " + shortDigest(digest)
	}
	return DependencyStatus{
		Name:      "runtime_image",
		Required:  true,
		Available: true,
		Degraded:  digest == "",
		Details:   detail,
	}
}

func shortDigest(digest string) string {
	d := strings.TrimPrefix(strings.TrimSpace(digest), "sha256:")
	if len(d) > 12 {
		return d[:12] + "…"
	}
	return d
}

// Provider probe deadlines (FR-39): per-provider 2 s, catalog overall 10 s,
// at most 4 providers probed concurrently. A probe that misses its deadline
// is recorded as "timeout" — the report schema is unchanged.
const (
	providerProbeTimeout     = 2 * time.Second
	providerCatalogDeadline  = 10 * time.Second
	providerProbeConcurrency = 4
)

// probeProviderCatalog probes every configured provider's health and model
// list concurrently (bounded), under a catalog-wide deadline. Ordering of
// the returned list matches the config declaration order regardless of
// probe completion order.
func probeProviderCatalog(ctx context.Context, defs []*model.ResolvedProvider, cfg Config, secrets config.Secrets) []ProviderHealth {
	if len(defs) == 0 {
		return nil
	}
	catalogCtx, cancel := context.WithTimeout(ctx, providerCatalogDeadline)
	defer cancel()

	results := make([]ProviderHealth, len(defs))
	g, gctx := errgroup.WithContext(catalogCtx)
	g.SetLimit(providerProbeConcurrency)
	for i := range defs {
		i, def := i, defs[i]
		g.Go(func() error {
			ph := ProviderHealth{
				Name:      def.Name,
				Kind:      def.Kind,
				Endpoint:  def.Endpoint,
				SetupHint: def.SetupHint,
			}
			if strings.EqualFold(def.Name, cfg.InferenceProvider) {
				ph.Selected = true
			}
			probeCtx, probeCancel := context.WithTimeout(gctx, providerProbeTimeout)
			defer probeCancel()
			applyProviderProbe(probeCtx, &ph, def, secrets)
			results[i] = ph
			return nil
		})
	}
	// The catalog deadline is enforced by the per-probe contexts; a straggler
	// past the overall deadline is recorded as timed out rather than blocking.
	_ = g.Wait()
	for i := range results {
		if results[i].State == "" {
			results[i].State = "timeout"
		}
	}
	return results
}

// applyProviderProbe runs one provider's health and model-list probes under
// the caller's (already bounded) context.
func applyProviderProbe(ctx context.Context, ph *ProviderHealth, def *model.ResolvedProvider, secrets config.Secrets) {
	pcfg := llm.ProviderConfig{
		Provider: def.Name,
		Kind:     def.Kind,
		Endpoint: def.Endpoint,
	}
	pbe, pberr := llm.New(pcfg, llm.ProviderSecrets{APIKey: secrets.LLMAPIKey})
	if pberr != nil {
		ph.State = "unhealthy"
		ph.Error = pberr.Error()
		return
	}
	defer func() { _ = pbe.Close() }()
	phState, phErr := pbe.Health(ctx)
	// A probe cut down by its own deadline (or the catalog deadline) is a
	// timeout regardless of how the backend spelled the failure — the
	// deadline is this probe's verdict, not the backend's health.
	if phErr != nil && ctx.Err() != nil {
		ph.State = "timeout"
		return
	}
	if phErr != nil {
		ph.State = "unhealthy"
		ph.Error = phErr.Error()
		return
	}
	if phState != nil {
		ph.State = string(phState.State)
	}
	if models, modErr := pbe.ListModels(ctx); modErr == nil {
		for _, m := range models {
			ph.Models = append(ph.Models, m.Name)
		}
	}
}

// canonicalRecipeIDs is the seven-recipe canonical set (§5.7) plus the
// built-in clarification target: every family handoff must resolve to one of
// these, and doctor reports any that a workspace lacks.
var canonicalRecipeIDs = []string{
	"euclo.thoughtrecipe.default",
	"euclo.thoughtrecipe.code_review",
	"euclo.thoughtrecipe.investigation",
	"euclo.thoughtrecipe.debug_tdd_repair",
	"euclo.thoughtrecipe.dep_upgrade",
	"euclo.thoughtrecipe.test_synthesis",
	"euclo.thoughtrecipe.extract_func",
}

type recipesCheckResult struct {
	ready   bool
	errText string
	found   []string
	missing []string
}

// checkCanonicalRecipes reports the workspace's canonical recipe state:
// directory present, files parseable, every canonical ID registered. A
// workspace initialized before the canonical set ships fails closed with an
// actionable message (re-init), never silently.
func checkCanonicalRecipes(workspace string) recipesCheckResult {
	result := recipesCheckResult{}
	// Diagnostics have no live runtime registry; seed a static capability
	// view from the self-declared euclo set so `do relurpic:` references in
	// the canonical recipes resolve exactly as they would at run time.
	caps := capabilityregistry.NewRegistry()
	for _, capID := range eucloservices.EucloCapabilityIDs() {
		desc := capabilitydescriptor.CapabilityDescriptor{
			ID:           capID,
			Name:         capID,
			Kind:         capabilityagentspec.CapabilityKindTool,
			Availability: capabilitydescriptor.AvailabilitySpec{Available: true},
		}
		if err := caps.RegisterCapability(context.Background(), desc); err != nil {
			result.errText = fmt.Sprintf("seed capability view: %v", err)
			result.missing = append(result.missing, canonicalRecipeIDs...)
			return result
		}
	}
	loader := thoughtrecipes.NewLoader().WithCapabilityRegistry(eucloservices.CapabilityLookup(caps))
	loadResult, err := loader.LoadWorkspace(workspace)
	if err != nil {
		// Sources unreadable (missing directory, unreadable file): nothing
		// registered, so every canonical ID is missing.
		result.errText = fmt.Sprintf("read thoughtrecipe sources: %v (run 'relurpish doctor --fix' to materialize starter recipes)", err)
		result.missing = append(result.missing, canonicalRecipeIDs...)
		return result
	}
	if loadResult == nil || loadResult.Registry == nil {
		result.errText = "thoughtrecipe registry unavailable (run 'relurpish doctor --fix' to materialize starter recipes)"
		return result
	}
	for _, id := range canonicalRecipeIDs {
		if _, ok := loadResult.Registry.Get(id); ok {
			result.found = append(result.found, id)
		} else {
			result.missing = append(result.missing, id)
		}
	}
	if len(result.missing) > 0 {
		result.errText = fmt.Sprintf("missing canonical thoughtrecipes: %s (run 'relurpish doctor --fix' to materialize starter recipes)", strings.Join(result.missing, ", "))
		return result
	}
	result.ready = true
	return result
}
