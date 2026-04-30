package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/agents"
	appruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	manifest "codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
	"gopkg.in/yaml.v3"
)

func withCLIState(t *testing.T, ws string) {
	t.Helper()
	originalWorkspace := workspace
	originalCfgFile := cfgFile
	originalGlobalCfg := globalCfg
	t.Cleanup(func() {
		workspace = originalWorkspace
		cfgFile = originalCfgFile
		globalCfg = originalGlobalCfg
	})
	workspace = ws
	cfgFile = ""
	globalCfg = nil
}

func stubStartWorkspaceFn(t *testing.T, ws string, compiledPolicy bool) {
	t.Helper()
	origOpenWorkspace := openWorkspaceFn
	t.Cleanup(func() {
		openWorkspaceFn = origOpenWorkspace
	})
	openWorkspaceFn = func(ctx context.Context, cfg ayenitd.WorkspaceConfig) (*ayenitd.Workspace, error) {
		manifestPath := cfg.ManifestPath
		if manifestPath == "" {
			manifestPath = filepath.Join(ws, "relurpify_cfg", "agents", "testfu.yaml")
		}
		loaded, err := manifest.LoadAgentManifest(manifestPath)
		if err != nil {
			return nil, err
		}
		effectivePerms, err := manifest.ResolveEffectivePermissions(ws, loaded)
		if err != nil {
			return nil, err
		}
		hitl := authorization.NewHITLBroker(cfg.HITLTimeout)
		perms, err := authorization.NewPermissionManager(ws, &effectivePerms, nil, hitl)
		if err != nil {
			return nil, err
		}
		env := ayenitd.WorkspaceEnvironment{
			Config: &core.Config{
				Name:              cfg.AgentName,
				Model:             cfg.InferenceModel,
				MaxIterations:     cfg.MaxIterations,
				NativeToolCalling: loaded.Spec.Agent != nil && loaded.Spec.Agent.NativeToolCallingEnabled(),
				AgentSpec:         loaded.Spec.Agent,
				DebugLLM:          cfg.DebugLLM,
				DebugAgent:        cfg.DebugAgent,
			},
			Registry:          capability.NewRegistry(),
			PermissionManager: perms,
			IndexManager:      &ast.IndexManager{},
		}
		var compiled *manifest.CompiledPolicyBundle
		if compiledPolicy && loaded.Spec.Agent != nil {
			engine, err := authorization.FromAgentSpecWithConfig(loaded.Spec.Agent, loaded.Metadata.Name, perms)
			if err != nil {
				return nil, err
			}
			compiled = &manifest.CompiledPolicyBundle{
				AgentID: loaded.Metadata.Name,
				Spec:    loaded.Spec.Agent,
			}
			env.Registry.SetPolicyEngine(engine)
		}
		return &ayenitd.Workspace{
			Environment:    env,
			Registration:   &authorization.AgentRegistration{ID: loaded.Metadata.Name, Manifest: loaded, Permissions: perms, HITL: hitl, Policy: compiledEngine(compiled)},
			AgentSpec:      loaded.Spec.Agent,
			CompiledPolicy: compiled,
			SkillResults:   nil,
			ServiceManager: ayenitd.NewServiceManager(),
		}, nil
	}
}

func compiledEngine(bundle *manifest.CompiledPolicyBundle) authorization.PolicyEngine {
	if bundle == nil {
		return nil
	}
	return nil
}

func writeAgentManifestFixture(t *testing.T, ws, name string, includeAgent bool) string {
	t.Helper()
	path := filepath.Join(ws, "relurpify_cfg", "agents", name+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	m := manifest.AgentManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentManifest",
		Metadata: manifest.ManifestMetadata{
			Name:        name,
			Version:     "1.0.0",
			Description: "test agent",
		},
		Spec: manifest.ManifestSpec{
			Image:   "ghcr.io/example/runtime:latest",
			Runtime: "gvisor",
			Defaults: &manifest.ManifestDefaults{
				Permissions: &core.PermissionSet{
					FileSystem: []core.FileSystemPermission{{
						Action:        core.FileSystemRead,
						Path:          filepath.ToSlash(filepath.Join(ws, "**")),
						Justification: "read workspace",
					}},
				},
			},
		},
	}
	if includeAgent {
		m.Spec.Agent = &core.AgentRuntimeSpec{
			Mode:    core.AgentModePrimary,
			Version: "1.0.0",
			Model: core.AgentModelConfig{
				Provider:    "ollama",
				Name:        "qwen2.5-coder:14b",
				Temperature: 0.1,
				MaxTokens:   2048,
			},
		}
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeSkillManifestFixture(t *testing.T, ws, name string, bins ...string) string {
	t.Helper()
	root := filepath.Join(ws, "relurpify_cfg", "skills", name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	m := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata: manifest.ManifestMetadata{
			Name:        name,
			Version:     "1.0.0",
			Description: "test skill",
		},
		Spec: manifest.SkillSpec{
			Requires: manifest.SkillRequiresSpec{Bins: bins},
		},
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "skill.manifest.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTestsuiteFixture(t *testing.T, ws, name string, cases []agenttest.CaseSpec, models []agenttest.ModelSpec) string {
	t.Helper()
	path := filepath.Join(ws, "testsuite", "agenttests", name+".testsuite.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	suite := agenttest.Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata: agenttest.SuiteMeta{
			Name: name,
		},
		Spec: agenttest.SuiteSpec{
			AgentName: "testfu",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Cases:     cases,
			Models:    models,
		},
	}
	data, err := yaml.Marshal(suite)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUtilityHelpersAndDefaults(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	withCLIState(t, "")

	if got := ensureWorkspace(); got != dir {
		t.Fatalf("ensureWorkspace = %q, want %q", got, dir)
	}
	if got := sessionDir(); got != filepath.Join(dir, "relurpify_cfg", "sessions") {
		t.Fatalf("sessionDir = %q", got)
	}
	if got := sanitizeName("  Hello World  "); got != "hello_world" {
		t.Fatalf("sanitizeName = %q", got)
	}
	if got := parseValue("true"); got != true {
		t.Fatalf("parseValue bool = %#v", got)
	}
	if got := parseValue("42"); got != int64(42) {
		t.Fatalf("parseValue int = %#v", got)
	}
	if got := parseValue("3.5"); got != 3.5 {
		t.Fatalf("parseValue float = %#v", got)
	}
	if got := parseValue("keep"); got != "keep" {
		t.Fatalf("parseValue string = %#v", got)
	}
	if got := prettyValue([]interface{}{1, "two"}); got != "[1, two]" {
		t.Fatalf("prettyValue slice = %q", got)
	}
	if got := prettyValue(map[string]interface{}{"a": 1}); !strings.Contains(got, "a: 1") {
		t.Fatalf("prettyValue map = %q", got)
	}
	if got := prettyValue(7); got != "7" {
		t.Fatalf("prettyValue scalar = %q", got)
	}
	if got := defaultModelName(); got != "codellama:13b" {
		t.Fatalf("defaultModelName = %q", got)
	}
	globalCfg = &manifest.GlobalConfig{DefaultModel: manifest.ModelRef{Name: "test-model"}}
	if got := defaultModelName(); got != "test-model" {
		t.Fatalf("defaultModelName with cfg = %q", got)
	}
	t.Setenv("OLLAMA_HOST", "http://example.invalid:11434")
	if got := defaultEndpoint(); got != "http://example.invalid:11434" {
		t.Fatalf("defaultEndpoint = %q", got)
	}
	t.Setenv("OLLAMA_HOST", "")
	if got := defaultEndpoint(); got != "http://localhost:11434" {
		t.Fatalf("defaultEndpoint fallback = %q", got)
	}

	missing := filepath.Join(dir, "does-not-exist.yaml")
	m, err := readConfigMap(missing)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 0 {
		t.Fatalf("expected empty config map, got %#v", m)
	}
	cfgPath := filepath.Join(dir, "manifest.yaml")
	cfgData := map[string]interface{}{"nested": map[string]interface{}{"value": 3}}
	if err := writeConfigMap(cfgPath, cfgData); err != nil {
		t.Fatal(err)
	}
	loaded, err := readConfigMap(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := getConfigValue(loaded, "nested.value")
	if !ok || value != 3 {
		t.Fatalf("getConfigValue = (%v, %v)", value, ok)
	}
	if _, ok := getConfigValue(loaded, "nested.missing"); ok {
		t.Fatal("expected missing config key")
	}
}

func TestRegistryAndAgentCommands(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)
	badPath := filepath.Join(ws, "relurpify_cfg", "agents", "broken.yaml")
	if err := os.WriteFile(badPath, []byte("not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := buildRegistry(ws)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("testfu"); !ok {
		t.Fatal("expected testfu manifest in registry")
	}
	if got := selectDefaultAgent(reg); got != "testfu" {
		t.Fatalf("selectDefaultAgent = %q", got)
	}
	if got := selectDefaultAgent(newAgentRegistry()); got != "coding" {
		t.Fatalf("selectDefaultAgent empty = %q", got)
	}

	listCmd := newAgentsListCmd()
	var listOut bytes.Buffer
	listCmd.SetOut(&listOut)
	if err := listCmd.RunE(listCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listOut.String(), "testfu (primary)") {
		t.Fatalf("list output missing agent summary: %q", listOut.String())
	}
	if !strings.Contains(listOut.String(), "Manifest load errors:") {
		t.Fatalf("list output missing error section: %q", listOut.String())
	}

	createCmd := newAgentsCreateCmd()
	if err := createCmd.Flags().Set("name", "Demo Agent"); err != nil {
		t.Fatal(err)
	}
	if err := createCmd.Flags().Set("description", "Created by test"); err != nil {
		t.Fatal(err)
	}
	var createOut bytes.Buffer
	createCmd.SetOut(&createOut)
	if err := createCmd.RunE(createCmd, nil); err != nil {
		t.Fatal(err)
	}
	createdPath := filepath.Join(ws, "relurpify_cfg", "agents", "demo_agent.yaml")
	data, err := os.ReadFile(createdPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "codellama:13b") {
		t.Fatalf("expected default model in created manifest: %s", string(data))
	}
	if !strings.Contains(createOut.String(), "Created") {
		t.Fatalf("create output missing confirmation: %q", createOut.String())
	}
	if err := createCmd.RunE(createCmd, nil); err == nil {
		t.Fatal("expected duplicate manifest error")
	}

	testCmd := newAgentsTestCmd()
	var testOut bytes.Buffer
	testCmd.SetOut(&testOut)
	if err := testCmd.RunE(testCmd, []string{"testfu"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(testOut.String(), "Manifest testfu valid") {
		t.Fatalf("unexpected test output: %q", testOut.String())
	}
	if err := testCmd.RunE(testCmd, []string{"missing"}); err == nil {
		t.Fatal("expected missing agent error")
	}

	emptyWS := t.TempDir()
	withCLIState(t, emptyWS)
	emptyList := newAgentsListCmd()
	var emptyOut bytes.Buffer
	emptyList.SetOut(&emptyOut)
	if err := emptyList.RunE(emptyList, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emptyOut.String(), "No agents found.") {
		t.Fatalf("expected empty registry message: %q", emptyOut.String())
	}
}

func TestConfigSessionCommands(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	cfgPath := filepath.Join(ws, "relurpify_cfg", "manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("permissions:\n  file_write: ask\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgFile = cfgPath

	getCmd := newConfigGetCmd()
	var out bytes.Buffer
	getCmd.SetOut(&out)
	if err := getCmd.RunE(getCmd, []string{"permissions.file_write"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "ask" {
		t.Fatalf("unexpected config get output: %q", out.String())
	}
	if err := getCmd.RunE(getCmd, []string{"permissions.missing"}); err == nil {
		t.Fatal("expected missing config key error")
	}

	setCmd := newConfigSetCmd()
	out.Reset()
	setCmd.SetOut(&out)
	if err := setCmd.RunE(setCmd, []string{"context.max_files", "10"}); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "max_files: 10") {
		t.Fatalf("config not updated: %s", string(updated))
	}

	saveCmd := newSessionSaveCmd()
	if err := saveCmd.Flags().Set("name", "Nightly Run"); err != nil {
		t.Fatal(err)
	}
	if err := saveCmd.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := saveCmd.Flags().Set("mode", "review"); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	saveCmd.SetOut(&out)
	if err := saveCmd.RunE(saveCmd, nil); err != nil {
		t.Fatal(err)
	}
	savedPath := filepath.Join(ws, "relurpify_cfg", "sessions", "nightly_run.yaml")
	if _, err := os.Stat(savedPath); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Session saved") {
		t.Fatalf("unexpected save output: %q", out.String())
	}

	loadCmd := newSessionLoadCmd()
	out.Reset()
	loadCmd.SetOut(&out)
	if err := loadCmd.RunE(loadCmd, []string{"nightly_run"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Session Nightly Run") {
		t.Fatalf("unexpected load output: %q", out.String())
	}
	if err := loadCmd.RunE(loadCmd, []string{"missing"}); err == nil {
		t.Fatal("expected missing session error")
	}

	listCmd := newSessionListCmd()
	out.Reset()
	listCmd.SetOut(&out)
	if err := listCmd.RunE(listCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Nightly Run") {
		t.Fatalf("unexpected list output: %q", out.String())
	}

	emptyWS := t.TempDir()
	withCLIState(t, emptyWS)
	emptyList := newSessionListCmd()
	out.Reset()
	emptyList.SetOut(&out)
	if err := emptyList.RunE(emptyList, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No saved sessions.") {
		t.Fatalf("expected empty session output, got %q", out.String())
	}
}

func TestSkillHelpersAndCommands(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)
	_ = writeSkillManifestFixture(t, ws, "demo-skill")

	if !containsSkill([]string{"alpha", "demo-skill"}, " demo-skill ") {
		t.Fatal("containsSkill should match trimmed names")
	}
	if containsSkill([]string{"alpha"}, "") {
		t.Fatal("containsSkill should reject empty name")
	}
	if got := manifestPathOrDefault(""); got != "default" {
		t.Fatalf("manifestPathOrDefault = %q", got)
	}
	if got := manifestPathOrDefault("/tmp/manifest.yaml"); got != "/tmp/manifest.yaml" {
		t.Fatalf("manifestPathOrDefault explicit = %q", got)
	}
	if effectiveAgentSpec(nil, nil) != nil {
		t.Fatal("expected nil effective spec")
	}
	manifestSpec := &manifest.AgentManifest{Spec: manifest.ManifestSpec{Agent: &core.AgentRuntimeSpec{Mode: core.AgentModePrimary}}}
	if got := effectiveAgentSpec(manifestSpec, nil); got != manifestSpec.Spec.Agent {
		t.Fatal("expected manifest agent spec")
	}
	contractSpec := &manifest.EffectiveAgentContract{AgentSpec: &core.AgentRuntimeSpec{Mode: core.AgentModeSub}}
	if got := effectiveAgentSpec(manifestSpec, contractSpec); got != contractSpec.AgentSpec {
		t.Fatal("expected contract agent spec")
	}

	root := filepath.Join(ws, "skill-root")
	skill := manifest.SkillManifest{
		Spec: manifest.SkillSpec{
			ResourcePaths: manifest.SkillResourceSpec{
				Scripts:   []string{""},
				Resources: []string{"resources", filepath.Join(ws, "abs-resources")},
				Templates: []string{"templates"},
			},
		},
	}
	if err := createSkillResourceDirs(root, skill); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "resources"),
		filepath.Join(root, "templates"),
		filepath.Join(ws, "abs-resources"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected resource dir %s: %v", path, err)
		}
	}

	initCmd := newSkillInitCmd()
	if err := initCmd.Flags().Set("description", "Custom skill"); err != nil {
		t.Fatal(err)
	}
	if err := initCmd.Flags().Set("version", "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := initCmd.Flags().Set("with-tests", "true"); err != nil {
		t.Fatal(err)
	}
	if err := initCmd.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	initCmd.SetOut(&out)
	if err := initCmd.RunE(initCmd, []string{"Demo Skill"}); err != nil {
		t.Fatal(err)
	}
	initPath := filepath.Join(ws, "relurpify_cfg", "skills", "Demo Skill", "skill.manifest.yaml")
	if _, err := os.Stat(initPath); err != nil {
		t.Fatalf("expected skill manifest at %s: %v", initPath, err)
	}
	if !strings.Contains(out.String(), "initialized") {
		t.Fatalf("unexpected init output: %q", out.String())
	}

	validateCmd := newSkillValidateCmd()
	out.Reset()
	validateCmd.SetOut(&out)
	if err := validateCmd.RunE(validateCmd, []string{"Demo Skill"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Skill Demo Skill valid") {
		t.Fatalf("unexpected validate output: %q", out.String())
	}

	doctorCmd := newSkillDoctorCmd()
	if err := doctorCmd.Flags().Set("manifest", filepath.Join(ws, "relurpify_cfg", "agents", "testfu.yaml")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	doctorCmd.SetOut(&out)
	if err := doctorCmd.RunE(doctorCmd, []string{"Demo Skill"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "compatible") {
		t.Fatalf("unexpected doctor output: %q", out.String())
	}

	badSkill := writeSkillManifestFixture(t, ws, "needs-bin", "definitely-not-installed")
	if _, err := os.Stat(badSkill); err != nil {
		t.Fatal(err)
	}
	badDoctor := newSkillDoctorCmd()
	if err := badDoctor.Flags().Set("manifest", filepath.Join(ws, "relurpify_cfg", "agents", "testfu.yaml")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	badDoctor.SetOut(&out)
	if err := badDoctor.RunE(badDoctor, []string{"needs-bin"}); err == nil {
		t.Fatal("expected missing binary error")
	}

	testCmd := newSkillTestCmd()
	if err := testCmd.Flags().Set("out", filepath.Join(ws, "out")); err != nil {
		t.Fatal(err)
	}
	if err := testCmd.RunE(testCmd, []string{"demo skill"}); err == nil {
		t.Fatal("expected missing testsuite error")
	}
}

func TestAgentTestHelpersAndLanes(t *testing.T) {
	if got, err := resolveAgentTestLane(""); err != nil || got != (agentTestLanePreset{}) {
		t.Fatalf("empty lane = (%+v, %v)", got, err)
	}
	if got, err := resolveAgentTestLane("pr-smoke"); err != nil || !got.Strict || got.Profile != "ci-live" || got.Tier != "smoke" {
		t.Fatalf("pr-smoke lane = (%+v, %v)", got, err)
	}
	if got, err := resolveAgentTestLane("merge-stable"); err != nil || !got.Strict || got.Tier != "stable" {
		t.Fatalf("merge-stable lane = (%+v, %v)", got, err)
	}
	if got, err := resolveAgentTestLane("quarantined-live"); err != nil || !got.IncludeQuarantined {
		t.Fatalf("quarantined-live lane = (%+v, %v)", got, err)
	}
	if _, err := resolveAgentTestLane("unknown"); err == nil {
		t.Fatal("expected unknown lane error")
	}

	suite := &agenttest.Suite{
		Metadata: agenttest.SuiteMeta{Quarantined: true, Tier: "stable"},
		Spec: agenttest.SuiteSpec{
			Execution: agenttest.SuiteExecutionSpec{Profile: "ci-live"},
		},
	}
	if shouldRunAgentTestSuite(suite, "stable", "ci-live", false) {
		t.Fatal("quarantined suite should be skipped by default")
	}
	if !shouldRunAgentTestSuite(suite, "stable", "ci-live", true) {
		t.Fatal("quarantined suite should run when included")
	}
	if shouldRunAgentTestSuite(suite, "smoke", "ci-live", true) {
		t.Fatal("tier mismatch should skip suite")
	}
	if shouldRunAgentTestSuite(suite, "stable", "replay", true) {
		t.Fatal("profile mismatch should skip suite")
	}

	if got := reportRunRoot(nil); got != "" {
		t.Fatalf("reportRunRoot nil = %q", got)
	}
	if got := reportRunRoot(&agenttest.SuiteReport{}); got != "" {
		t.Fatalf("reportRunRoot empty = %q", got)
	}
	root := reportRunRoot(&agenttest.SuiteReport{Cases: []agenttest.CaseReport{{ArtifactsDir: filepath.Join(t.TempDir(), "run", "artifacts", "case")}}})
	if root == "" {
		t.Fatal("expected reportRunRoot value")
	}

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	dst := filepath.Join(tmp, "dst.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("copyFile = %q", string(got))
	}

	report := &agenttest.SuiteReport{Cases: []agenttest.CaseReport{{Name: "b"}, {Name: "a"}}}
	selected := selectPromotableCases(report, "", true)
	if len(selected) != 2 || selected[0].Name != "a" {
		t.Fatalf("selectPromotableCases all = %+v", selected)
	}
	if got := selectPromotableCases(report, "b", false); len(got) != 1 || got[0].Name != "b" {
		t.Fatalf("selectPromotableCases single = %+v", got)
	}

	now := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)
	if got := formatRecordedAt(time.Time{}); got != "recorded unknown" {
		t.Fatalf("formatRecordedAt zero = %q", got)
	}
	if got := formatRecordedAt(now); got != "recorded 2026-03-18" {
		t.Fatalf("formatRecordedAt = %q", got)
	}
	inspection := &llm.TapeInspection{Header: &llm.TapeHeader{ModelName: "model"}, FirstRecordedAt: now.Add(-31 * 24 * time.Hour)}
	if got := formatTapeStatus(inspection, "model", now); !strings.Contains(got, "days old") {
		t.Fatalf("formatTapeStatus = %q", got)
	}
	if got := formatTapeStatus(inspection, "other", now); !strings.Contains(got, "model mismatch") {
		t.Fatalf("formatTapeStatus mismatch = %q", got)
	}
	if got := formatTapeStatus(&llm.TapeInspection{}, "model", now); got != "legacy tape" {
		t.Fatalf("formatTapeStatus legacy = %q", got)
	}
	suiteForModels := &agenttest.Suite{Spec: agenttest.SuiteSpec{Models: []agenttest.ModelSpec{{Name: "a"}, {Name: "b"}}}}
	if got := suiteModelsForCase(suiteForModels, agenttest.CaseSpec{}); len(got) != 2 {
		t.Fatalf("suiteModelsForCase fallback = %+v", got)
	}
	override := agenttest.CaseSpec{Overrides: agenttest.CaseOverrideSpec{Model: &agenttest.ModelSpec{Name: "override"}}}
	if got := suiteModelsForCase(suiteForModels, override); len(got) != 1 || got[0].Name != "override" {
		t.Fatalf("suiteModelsForCase override = %+v", got)
	}

	blankSuite := &agenttest.Suite{Spec: agenttest.SuiteSpec{}}
	if got := suiteModelsForCase(blankSuite, agenttest.CaseSpec{}); got != nil && len(got) != 0 {
		t.Fatalf("expected empty models, got %+v", got)
	}
}

func TestAgentTestFileHelpers(t *testing.T) {
	tmp := t.TempDir()
	headerTape := filepath.Join(tmp, "header.tape.jsonl")
	if err := os.WriteFile(headerTape, []byte(`{"kind":"_header","request":{"header":{"kind":"_header","model_name":"m"}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	header, err := readTapeHeader(headerTape)
	if err != nil {
		t.Fatal(err)
	}
	if header == nil || header.ModelName != "m" {
		t.Fatalf("unexpected tape header: %+v", header)
	}
	legacyTape := filepath.Join(tmp, "legacy.tape.jsonl")
	if err := os.WriteFile(legacyTape, []byte(`{"kind":"generate"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	header, err = readTapeHeader(legacyTape)
	if err != nil {
		t.Fatal(err)
	}
	if header != nil {
		t.Fatalf("expected nil legacy header, got %+v", header)
	}

	if got := sanitizeAgentTestTapeName("  Hello World! "); got != "Hello_World" {
		t.Fatalf("sanitizeAgentTestTapeName = %q", got)
	}
	if got := sanitizeAgentTestTapeName("!!!"); got != "unnamed" {
		t.Fatalf("sanitizeAgentTestTapeName fallback = %q", got)
	}
	if got := goldenTapeFilename("hello world", "qwen2.5-coder:14b"); got != "hello_world__qwen2_5_coder_14b.tape.jsonl" {
		t.Fatalf("goldenTapeFilename = %q", got)
	}

	reportPath := filepath.Join(tmp, "report.json")
	if err := os.WriteFile(reportPath, []byte(`{"cases":[{"name":"a"},{"name":"b"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := loadSuiteReport(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Cases) != 2 {
		t.Fatalf("loadSuiteReport = %+v", report)
	}
}

func TestDiscoverSuitesAndSuiteFilters(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	suitePath := writeTestsuiteFixture(t, ws, "alpha", []agenttest.CaseSpec{{Name: "smoke"}}, nil)
	if got := discoverSuites(ws, "alpha"); len(got) != 1 || got[0] != suitePath {
		t.Fatalf("discoverSuites canonical = %+v", got)
	}
	fallbackDir := manifest.New(ws).TestsuitesDir()
	if err := os.MkdirAll(fallbackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(ws, "testsuite")); err != nil {
		t.Fatal(err)
	}
	fallbackPath := filepath.Join(fallbackDir, "beta.testsuite.yaml")
	if err := os.WriteFile(fallbackPath, []byte(`apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: beta
spec:
  agent_name: testfu
  manifest: relurpify_cfg/agent.manifest.yaml
  cases:
    - name: smoke
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := discoverSuites(ws, "beta"); len(got) != 1 || got[0] != fallbackPath {
		t.Fatalf("discoverSuites fallback = %+v", got)
	}

	suite := &agenttest.Suite{Spec: agenttest.SuiteSpec{Cases: []agenttest.CaseSpec{{Name: "smoke", Tags: []string{"fast"}}, {Name: "long", Tags: []string{"slow"}}}}}
	filtered, err := filterAgentTestSuiteCases(suite, "", []string{"fast"})
	if err != nil || len(filtered.Spec.Cases) != 1 || filtered.Spec.Cases[0].Name != "smoke" {
		t.Fatalf("filter by tag = %+v, %v", filtered, err)
	}
	_, err = filterAgentTestSuiteCases(nil, "", nil)
	if err == nil {
		t.Fatal("expected nil suite error")
	}
	_, err = filterAgentTestSuiteCases(&agenttest.Suite{}, "", nil)
	if err == nil {
		t.Fatal("expected empty suite error")
	}
	_, err = filterAgentTestSuiteCases(suite, "missing", nil)
	if err == nil {
		t.Fatal("expected missing case error")
	}
}

type stubWorkflowExecutor struct {
	initCalled    bool
	executeCalled bool
}

func (s *stubWorkflowExecutor) Initialize(config *core.Config) error {
	s.initCalled = true
	return nil
}

func (s *stubWorkflowExecutor) Execute(ctx context.Context, task *core.Task, state *contextdata.Envelope) (*core.Result, error) {
	s.executeCalled = true
	return &core.Result{NodeID: "done", Data: map[string]any{"status": "ok"}}, nil
}

func (s *stubWorkflowExecutor) Capabilities() []string { return nil }

func (s *stubWorkflowExecutor) BuildGraph(task *core.Task) (*graph.Graph, error) {
	return nil, nil
}

type fakeAgentTestRunner struct {
	report *agenttest.SuiteReport
	err    error
}

func (f *fakeAgentTestRunner) RunSuite(ctx context.Context, suite *agenttest.Suite, opts agenttest.RunOptions) (*agenttest.SuiteReport, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.report != nil {
		return f.report, nil
	}
	artifactDir := filepath.Join(opts.OutputDir, "artifacts", "case__model")
	return &agenttest.SuiteReport{
		SuitePath: suite.SourcePath,
		Profile:   opts.Profile,
		Strict:    opts.Strict,
		Cases: []agenttest.CaseReport{{
			Name:         suite.Spec.Cases[0].Name,
			Model:        "model",
			Success:      true,
			ArtifactsDir: artifactDir,
		}},
		PassedCases: 1,
	}, nil
}

func TestStartCmdCoveragePaths(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	cmd := newStartCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ready in") {
		t.Fatalf("unexpected no-instruction output: %q", out.String())
	}

	out.Reset()
	if err := cmd.Flags().Set("instruction", "inspect the workspace"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Dry run: testfu in") {
		t.Fatalf("unexpected dry-run output: %q", out.String())
	}

	origRegisterAgent := registerAgentFn
	origRegisterProviders := registerBuiltinProvidersFn
	origRegisterRelurpic := registerBuiltinRelurpicCapabilitiesFn
	origRegisterAgentCaps := registerAgentCapabilitiesFn
	origBuildFromSpec := buildFromSpecFn
	t.Cleanup(func() {
		registerAgentFn = origRegisterAgent
		registerBuiltinProvidersFn = origRegisterProviders
		registerBuiltinRelurpicCapabilitiesFn = origRegisterRelurpic
		registerAgentCapabilitiesFn = origRegisterAgentCaps
		buildFromSpecFn = origBuildFromSpec
	})
	globalCfg = &manifest.GlobalConfig{
		Logging: manifest.LoggingConfig{LLM: true, Agent: true},
	}
	stubStartWorkspaceFn(t, ws, true)

	registerAgentFn = func(ctx context.Context, cfg authorization.RuntimeConfig) (*authorization.AgentRegistration, error) {
		manifestPath := filepath.Join(ws, "relurpify_cfg", "agents", "testfu.yaml")
		loaded, err := manifest.LoadAgentManifest(manifestPath)
		if err != nil {
			return nil, err
		}
		effectivePerms, err := manifest.ResolveEffectivePermissions(ws, loaded)
		if err != nil {
			return nil, err
		}
		perms, err := authorization.NewPermissionManager(ws, &effectivePerms, nil, nil)
		if err != nil {
			return nil, err
		}
		return &authorization.AgentRegistration{
			ID:          loaded.Metadata.Name,
			Manifest:    loaded,
			Permissions: perms,
			Runtime:     nil,
			HITL:        authorization.NewHITLBroker(0),
		}, nil
	}
	registerBuiltinProvidersFn = func(ctx context.Context, rt *appruntime.Runtime) error { return nil }
	registerBuiltinRelurpicCapabilitiesFn = func(registry *capability.Registry, model core.LanguageModel, cfg *core.Config, opts ...agents.BuiltinRelurpicOption) error {
		return nil
	}
	registerAgentCapabilitiesFn = func(registry *capability.Registry, env agents.AgentEnvironment) error { return nil }
	fakeAgent := &stubWorkflowExecutor{}
	buildFromSpecFn = func(env *agentenv.WorkspaceEnvironment, spec core.AgentRuntimeSpec) (graph.WorkflowExecutor, error) {
		return fakeAgent, nil
	}

	out.Reset()
	cmd2 := newStartCmd()
	cmd2.SetOut(&out)
	if err := cmd2.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.Flags().Set("instruction", "change something"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.Flags().Set("resume-latest-workflow", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.Flags().Set("workflow", "wf-123"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.Flags().Set("rerun-from-step", "step-9"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.RunE(cmd2, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Agent complete") {
		t.Fatalf("unexpected execution output: %q", out.String())
	}

	out.Reset()
	cmd3 := newStartCmd()
	cmd3.SetOut(&out)
	if err := cmd3.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := cmd3.Flags().Set("instruction", "sandboxed execution"); err != nil {
		t.Fatal(err)
	}
	if err := cmd3.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd3.RunE(cmd3, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Agent complete") {
		t.Fatalf("unexpected sandboxed execution output: %q", out.String())
	}
}

func TestStartCmdErrorBranches(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)
	writeAgentManifestFixture(t, ws, "specless", false)

	cmd := newStartCmd()
	if err := cmd.Flags().Set("agent", "missing-agent"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("instruction", "do something"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "agent missing-agent not found") {
		t.Fatalf("expected missing-agent error, got %v", err)
	}

	cmd2 := newStartCmd()
	if err := cmd2.Flags().Set("agent", "specless"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.Flags().Set("instruction", "do something"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.RunE(cmd2, nil); err == nil || !strings.Contains(err.Error(), "missing spec.agent section") {
		t.Fatalf("expected missing-spec error, got %v", err)
	}
}

func TestStartCmdPolicyAndBuildFallbackBranches(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	origRegisterAgent := registerAgentFn
	origRegisterProviders := registerBuiltinProvidersFn
	origRegisterRelurpic := registerBuiltinRelurpicCapabilitiesFn
	origRegisterAgentCaps := registerAgentCapabilitiesFn
	origBuildFromSpec := buildFromSpecFn
	t.Cleanup(func() {
		registerAgentFn = origRegisterAgent
		registerBuiltinProvidersFn = origRegisterProviders
		registerBuiltinRelurpicCapabilitiesFn = origRegisterRelurpic
		registerAgentCapabilitiesFn = origRegisterAgentCaps
		buildFromSpecFn = origBuildFromSpec
	})
	stubStartWorkspaceFn(t, ws, true)
	registerAgentFn = func(ctx context.Context, cfg authorization.RuntimeConfig) (*authorization.AgentRegistration, error) {
		loaded, err := manifest.LoadAgentManifest(filepath.Join(ws, "relurpify_cfg", "agents", "testfu.yaml"))
		if err != nil {
			return nil, err
		}
		effectivePerms, err := manifest.ResolveEffectivePermissions(ws, loaded)
		if err != nil {
			return nil, err
		}
		perms, err := authorization.NewPermissionManager(ws, &effectivePerms, nil, nil)
		if err != nil {
			return nil, err
		}
		return &authorization.AgentRegistration{ID: loaded.Metadata.Name, Manifest: loaded, Permissions: perms}, nil
	}
	registerBuiltinProvidersFn = func(ctx context.Context, rt *appruntime.Runtime) error { return nil }
	registerBuiltinRelurpicCapabilitiesFn = func(registry *capability.Registry, model core.LanguageModel, cfg *core.Config, opts ...agents.BuiltinRelurpicOption) error {
		return nil
	}
	registerAgentCapabilitiesFn = func(registry *capability.Registry, env agents.AgentEnvironment) error { return nil }
	buildCalls := 0
	buildFromSpecFn = func(env *agentenv.WorkspaceEnvironment, spec core.AgentRuntimeSpec) (graph.WorkflowExecutor, error) {
		buildCalls++
		if buildCalls == 1 {
			return nil, fmt.Errorf("synthetic build failure")
		}
		return &stubWorkflowExecutor{}, nil
	}

	cmd := newStartCmd()
	if err := cmd.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("instruction", "check build fallback"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}

	origOpenWorkspace := openWorkspaceFn
	t.Cleanup(func() { openWorkspaceFn = origOpenWorkspace })
	openWorkspaceFn = func(ctx context.Context, cfg ayenitd.WorkspaceConfig) (*ayenitd.Workspace, error) {
		loaded, err := manifest.LoadAgentManifest(filepath.Join(ws, "relurpify_cfg", "agents", "testfu.yaml"))
		if err != nil {
			return nil, err
		}
		effectivePerms, err := manifest.ResolveEffectivePermissions(ws, loaded)
		if err != nil {
			return nil, err
		}
		hitl := authorization.NewHITLBroker(cfg.HITLTimeout)
		perms, err := authorization.NewPermissionManager(ws, &effectivePerms, nil, hitl)
		if err != nil {
			return nil, err
		}
		return &ayenitd.Workspace{
			Environment: ayenitd.WorkspaceEnvironment{
				Config: &core.Config{
					Name:              cfg.AgentName,
					Model:             cfg.InferenceModel,
					MaxIterations:     cfg.MaxIterations,
					NativeToolCalling: loaded.Spec.Agent.NativeToolCallingEnabled(),
					AgentSpec:         loaded.Spec.Agent,
					DebugLLM:          cfg.DebugLLM,
					DebugAgent:        cfg.DebugAgent,
				},
				Registry:          capability.NewRegistry(),
				PermissionManager: perms,
				IndexManager:      &ast.IndexManager{},
			},
			Registration:   &authorization.AgentRegistration{ID: loaded.Metadata.Name, Manifest: loaded, Permissions: perms, HITL: hitl},
			AgentSpec:      loaded.Spec.Agent,
			ServiceManager: ayenitd.NewServiceManager(),
		}, nil
	}
	cmd2 := newStartCmd()
	if err := cmd2.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.Flags().Set("instruction", "check missing policy"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.RunE(cmd2, nil); err == nil || !strings.Contains(err.Error(), "compiled policy missing") {
		t.Fatalf("expected missing compiled policy error, got %v", err)
	}
}

func TestStartCmdDefaultAgentAndModeBranches(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	cmd := newStartCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("instruction", "dry run please"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("mode", ""); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Agent testfu ready in") && !strings.Contains(out.String(), "Dry run: testfu in") {
		t.Fatalf("unexpected default-agent output: %q", out.String())
	}
}

func TestStartCmdSetupErrorBranches(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	origOpenWorkspace := openWorkspaceFn
	t.Cleanup(func() { openWorkspaceFn = origOpenWorkspace })

	openWorkspaceFn = func(ctx context.Context, cfg ayenitd.WorkspaceConfig) (*ayenitd.Workspace, error) {
		return nil, fmt.Errorf("register failed")
	}

	cmd := newStartCmd()
	if err := cmd.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("instruction", "do work"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "register failed") {
		t.Fatalf("expected register failure, got %v", err)
	}

	openWorkspaceFn = func(ctx context.Context, cfg ayenitd.WorkspaceConfig) (*ayenitd.Workspace, error) {
		return nil, fmt.Errorf("sandbox failed")
	}
	cmd2 := newStartCmd()
	if err := cmd2.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.Flags().Set("instruction", "do work"); err != nil {
		t.Fatal(err)
	}
	if err := cmd2.RunE(cmd2, nil); err == nil || !strings.Contains(err.Error(), "sandbox failed") {
		t.Fatalf("expected sandbox failure, got %v", err)
	}

	openWorkspaceFn = func(ctx context.Context, cfg ayenitd.WorkspaceConfig) (*ayenitd.Workspace, error) {
		return nil, fmt.Errorf("memory failed")
	}
	cmd3 := newStartCmd()
	if err := cmd3.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := cmd3.Flags().Set("instruction", "do work"); err != nil {
		t.Fatal(err)
	}
	if err := cmd3.RunE(cmd3, nil); err == nil || !strings.Contains(err.Error(), "memory failed") {
		t.Fatalf("expected memory failure, got %v", err)
	}
}

func TestStartCmdProviderRegistrationError(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	origRegisterAgent := registerAgentFn
	origRegisterProviders := registerBuiltinProvidersFn
	origBuildFromSpec := buildFromSpecFn
	t.Cleanup(func() {
		registerAgentFn = origRegisterAgent
		registerBuiltinProvidersFn = origRegisterProviders
		buildFromSpecFn = origBuildFromSpec
	})

	registerAgentFn = func(ctx context.Context, cfg authorization.RuntimeConfig) (*authorization.AgentRegistration, error) {
		loaded, err := manifest.LoadAgentManifest(filepath.Join(ws, "relurpify_cfg", "agents", "testfu.yaml"))
		if err != nil {
			return nil, err
		}
		effectivePerms, err := manifest.ResolveEffectivePermissions(ws, loaded)
		if err != nil {
			return nil, err
		}
		perms, err := authorization.NewPermissionManager(ws, &effectivePerms, nil, nil)
		if err != nil {
			return nil, err
		}
		return &authorization.AgentRegistration{ID: loaded.Metadata.Name, Manifest: loaded, Permissions: perms}, nil
	}
	registerBuiltinProvidersFn = func(ctx context.Context, rt *appruntime.Runtime) error {
		return fmt.Errorf("provider registration failed")
	}
	buildFromSpecFn = func(env *agentenv.WorkspaceEnvironment, spec core.AgentRuntimeSpec) (graph.WorkflowExecutor, error) {
		return &stubWorkflowExecutor{}, nil
	}
	stubStartWorkspaceFn(t, ws, true)

	cmd := newStartCmd()
	if err := cmd.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("instruction", "do work"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "provider registration failed") {
		t.Fatalf("expected provider registration failure, got %v", err)
	}
}

func TestStartCmdInteractiveHitlBranch(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	origRegisterAgent := registerAgentFn
	origRegisterProviders := registerBuiltinProvidersFn
	origBuildFromSpec := buildFromSpecFn
	t.Cleanup(func() {
		registerAgentFn = origRegisterAgent
		registerBuiltinProvidersFn = origRegisterProviders
		buildFromSpecFn = origBuildFromSpec
	})

	registerAgentFn = func(ctx context.Context, cfg authorization.RuntimeConfig) (*authorization.AgentRegistration, error) {
		loaded, err := manifest.LoadAgentManifest(filepath.Join(ws, "relurpify_cfg", "agents", "testfu.yaml"))
		if err != nil {
			return nil, err
		}
		effectivePerms, err := manifest.ResolveEffectivePermissions(ws, loaded)
		if err != nil {
			return nil, err
		}
		perms, err := authorization.NewPermissionManager(ws, &effectivePerms, nil, nil)
		if err != nil {
			return nil, err
		}
		return &authorization.AgentRegistration{
			ID:          loaded.Metadata.Name,
			Manifest:    loaded,
			Permissions: perms,
			HITL:        authorization.NewHITLBroker(0),
		}, nil
	}
	registerBuiltinProvidersFn = func(ctx context.Context, rt *appruntime.Runtime) error { return nil }
	buildFromSpecFn = func(env *agentenv.WorkspaceEnvironment, spec core.AgentRuntimeSpec) (graph.WorkflowExecutor, error) {
		return &stubWorkflowExecutor{}, nil
	}
	stubStartWorkspaceFn(t, ws, true)

	cmd := newStartCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("instruction", "interactive branch"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Agent complete") {
		t.Fatalf("unexpected interactive output: %q", out.String())
	}
}

func TestAgentTestRunCmdWithStubRunner(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	suitePath := filepath.Join(ws, "testsuite", "agenttests", "alpha.testsuite.yaml")
	if err := os.MkdirAll(filepath.Dir(suitePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(suitePath, []byte(`apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: alpha
  tier: smoke
spec:
  agent_name: testfu
  manifest: relurpify_cfg/agent.manifest.yaml
  execution:
    profile: ci-live
  workspace:
    strategy: derived
    template_profile: default
  models:
    - name: model
  cases:
    - name: smoke
      task_type: analysis
      prompt: hello
      tags: [smoke]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := newAgentTestRunnerFn
	t.Cleanup(func() { newAgentTestRunnerFn = orig })
	newAgentTestRunnerFn = func() agentTestRunner {
		return &fakeAgentTestRunner{
			report: &agenttest.SuiteReport{
				SuitePath:   suitePath,
				Profile:     "ci-live",
				Strict:      false,
				PassedCases: 1,
				Cases: []agenttest.CaseReport{{
					Name:         "smoke",
					Model:        "model",
					Success:      true,
					ArtifactsDir: filepath.Join(ws, "relurpify_cfg", "test_runs", "alpha", "run-1", "artifacts", "smoke__model"),
				}},
			},
		}
	}

	cmd := newAgentTestRunCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("suite", suitePath); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "alpha.testsuite.yaml [ci-live]: 1/1 passed") {
		t.Fatalf("unexpected run output: %q", out.String())
	}
}

func TestAgentTestRunCmdStrictFailure(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	suitePath := filepath.Join(ws, "testsuite", "agenttests", "alpha.testsuite.yaml")
	if err := os.MkdirAll(filepath.Dir(suitePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(suitePath, []byte(`apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: alpha
  tier: smoke
spec:
  agent_name: testfu
  manifest: relurpify_cfg/agent.manifest.yaml
  execution:
    profile: ci-live
  workspace:
    strategy: derived
    template_profile: default
  cases:
    - name: smoke
      prompt: hello
`), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := newAgentTestRunnerFn
	t.Cleanup(func() { newAgentTestRunnerFn = orig })
	newAgentTestRunnerFn = func() agentTestRunner {
		return &fakeAgentTestRunner{
			report: &agenttest.SuiteReport{
				SuitePath:   suitePath,
				Profile:     "ci-live",
				Strict:      true,
				PassedCases: 0,
				FailedCases: 1,
				Cases: []agenttest.CaseReport{{
					Name:         "smoke",
					Success:      false,
					ArtifactsDir: filepath.Join(ws, "relurpify_cfg", "test_runs", "alpha", "run-1", "artifacts", "smoke__model"),
				}},
			},
		}
	}

	cmd := newAgentTestRunCmd()
	if err := cmd.Flags().Set("suite", suitePath); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("strict", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("profile", "ci-live"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "strict mode") {
		t.Fatalf("expected strict failure, got %v", err)
	}
}

func TestAgentTestRefreshCmdWithStubRunner(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	suitePath := writeTestsuiteFixture(t, ws, "alpha", []agenttest.CaseSpec{{Name: "smoke", Prompt: "hello"}}, []agenttest.ModelSpec{{Name: "model"}})
	runRoot := filepath.Join(ws, "relurpify_cfg", "test_runs", "alpha", "run-1", "artifacts")
	artifactDir := filepath.Join(runRoot, "smoke__model")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "tape.jsonl"), []byte(`{"kind":"_header","request":{"header":{"kind":"_header","model_name":"model"}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "interaction.tape.jsonl"), []byte(`{"kind":"generate"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := agenttest.SuiteReport{
		SuitePath:   suitePath,
		Profile:     "live",
		PassedCases: 1,
		Cases: []agenttest.CaseReport{{
			Name:         "smoke",
			Model:        "model",
			Success:      true,
			ArtifactsDir: artifactDir,
			FinishedAt:   time.Now().UTC(),
		}},
	}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runRoot, "report.json"), reportBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	orig := newAgentTestRunnerFn
	t.Cleanup(func() { newAgentTestRunnerFn = orig })
	newAgentTestRunnerFn = func() agentTestRunner {
		return &fakeAgentTestRunner{report: &report}
	}

	cmd := newAgentTestRefreshCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("suite", suitePath); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "promoted") {
		t.Fatalf("unexpected refresh output: %q", out.String())
	}
}

func TestAgentTestTapesCmd(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	suitePath := writeTestsuiteFixture(t, ws, "alpha", []agenttest.CaseSpec{{Name: "smoke", Prompt: "hello"}}, []agenttest.ModelSpec{{Name: "model"}})
	tapeDir := filepath.Join(ws, "testsuite", "agenttests", "tapes", "alpha")
	if err := os.MkdirAll(tapeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tapeDir, "smoke__model.tape.jsonl"), []byte(`{"kind":"_header","request":{"header":{"kind":"_header","model_name":"model","recorded_at":"2026-02-01T00:00:00Z"}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newAgentTestTapesCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("suite", suitePath); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Suite: alpha") {
		t.Fatalf("unexpected tapes output: %q", out.String())
	}
}

func TestSkillTestCmdWithStubRunner(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	agentManifestPath := writeAgentManifestFixture(t, ws, "testfu", true)
	agentManifestCopy := filepath.Join(ws, "relurpify_cfg", "agent.manifest.yaml")
	data, err := os.ReadFile(agentManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(agentManifestCopy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentManifestCopy, data, 0o644); err != nil {
		t.Fatal(err)
	}
	skillRoot := filepath.Join(ws, "relurpify_cfg", "skills", "demo-skill")
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	suitePath := filepath.Join(skillRoot, "testsuite.yaml")
	if err := os.WriteFile(suitePath, []byte(fmt.Sprintf(`apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: demo-skill
spec:
  agent_name: testfu
  manifest: %s
  workspace:
    strategy: derived
    template_profile: default
  cases:
    - name: skill-smoke
      task_type: analysis
      prompt: hello
`, filepath.ToSlash(agentManifestCopy))), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := newAgentTestRunnerFn
	t.Cleanup(func() { newAgentTestRunnerFn = orig })
	newAgentTestRunnerFn = func() agentTestRunner {
		return &fakeAgentTestRunner{
			report: &agenttest.SuiteReport{
				SuitePath:   suitePath,
				Profile:     "live",
				PassedCases: 1,
				Cases: []agenttest.CaseReport{{
					Name:         "skill-smoke",
					Success:      true,
					ArtifactsDir: filepath.Join(ws, "relurpify_cfg", "test_runs", "demo-skill", "run-1", "artifacts", "skill-smoke__model"),
				}},
			},
		}
	}

	cmd := newSkillTestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("out", filepath.Join(ws, "out")); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, []string{"demo-skill"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skillRoot, "testsuite.manifest.yaml")); err != nil {
		t.Fatalf("expected derived skill manifest: %v", err)
	}
}

func TestAgentTestPromoteCmd(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	suitePath := filepath.Join(ws, "testsuite", "agenttests", "alpha.testsuite.yaml")
	if err := os.MkdirAll(filepath.Dir(suitePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(suitePath, []byte(`apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: alpha
spec:
  agent_name: testfu
  manifest: relurpify_cfg/agent.manifest.yaml
  cases:
    - name: smoke
      prompt: hello
`), 0o644); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(ws, "relurpify_cfg", "test_runs", "alpha", "run-1")
	artifactsDir := filepath.Join(runDir, "artifacts", "smoke__model")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "tape.jsonl"), []byte(`{"kind":"_header","request":{"header":{"kind":"_header","model_name":"model"}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "interaction.tape.jsonl"), []byte(`{"kind":"generate"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := agenttest.SuiteReport{
		Cases: []agenttest.CaseReport{{
			Name:         "smoke",
			Model:        "model",
			Success:      true,
			ArtifactsDir: artifactsDir,
			FinishedAt:   time.Now().UTC(),
		}},
	}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), reportBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newAgentTestPromoteCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("suite", suitePath); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("run", runDir); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("case", "smoke"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "promoted") {
		t.Fatalf("unexpected promote output: %q", out.String())
	}
}

func TestPromoteAgentTestRunErrorBranches(t *testing.T) {
	ws := t.TempDir()
	suitePath := filepath.Join(ws, "testsuite", "agenttests", "alpha.testsuite.yaml")
	if err := os.MkdirAll(filepath.Dir(suitePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(suitePath, []byte(`apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: alpha
spec:
  agent_name: testfu
  manifest: relurpify_cfg/agent.manifest.yaml
  cases:
    - name: smoke
      prompt: hello
`), 0o644); err != nil {
		t.Fatal(err)
	}

	runDir := filepath.Join(ws, "relurpify_cfg", "test_runs", "alpha", "run-1")
	artifactsDir := filepath.Join(runDir, "artifacts", "smoke__model")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := agenttest.SuiteReport{
		Cases: []agenttest.CaseReport{{
			Name:         "smoke",
			Model:        "model",
			Success:      true,
			ArtifactsDir: artifactsDir,
			FinishedAt:   time.Now().UTC(),
		}},
	}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), reportBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "tape.jsonl"), []byte(`{"kind":"generate"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := promoteAgentTestRun(ws, suitePath, runDir, "missing", false, &out); err == nil {
		t.Fatal("expected no promotable cases error")
	}
	if err := promoteAgentTestRun(ws, suitePath, runDir, "smoke", false, &out); err == nil || !strings.Contains(err.Error(), "tape has no header") {
		t.Fatalf("expected no-header error, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "tape.jsonl"), []byte(`{"kind":"_header","request":{"header":{"kind":"_header","model_name":"other"}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := promoteAgentTestRun(ws, suitePath, runDir, "smoke", false, &out); err == nil || !strings.Contains(err.Error(), "does not match report model") {
		t.Fatalf("expected model mismatch error, got %v", err)
	}
}

func TestSkillDoctorCmdUsingAgentRegistry(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)
	skillRoot := filepath.Join(ws, "relurpify_cfg", "skills", "demo-skill")
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	skillManifest := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata: manifest.ManifestMetadata{
			Name: "demo-skill",
		},
		Spec: manifest.SkillSpec{},
	}
	if err := createSkillResourceDirs(skillRoot, skillManifest); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(skillRoot, "skill.manifest.yaml")
	skillData, err := yaml.Marshal(skillManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, skillData, 0o644); err != nil {
		t.Fatal(err)
	}
	agentManifestCopy := filepath.Join(ws, "relurpify_cfg", "agent.manifest.yaml")
	agentData, err := os.ReadFile(filepath.Join(ws, "relurpify_cfg", "agents", "testfu.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentManifestCopy, agentData, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newSkillDoctorCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, []string{"demo-skill"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "compatible") {
		t.Fatalf("unexpected doctor output: %q", out.String())
	}
}

func TestSkillDoctorCmdDefaultManifestPath(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	skillRoot := filepath.Join(ws, "relurpify_cfg", "skills", "demo-skill")
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	skillManifest := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata: manifest.ManifestMetadata{
			Name: "demo-skill",
		},
		Spec: manifest.SkillSpec{},
	}
	if err := createSkillResourceDirs(skillRoot, skillManifest); err != nil {
		t.Fatal(err)
	}
	skillData, err := yaml.Marshal(skillManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "skill.manifest.yaml"), skillData, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newSkillDoctorCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, []string{"demo-skill"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "manifest=default") {
		t.Fatalf("unexpected default-manifest output: %q", out.String())
	}
}

func TestSkillDoctorCmdBinaryPresent(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	skillRoot := filepath.Join(ws, "relurpify_cfg", "skills", "demo-skill")
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	skillManifest := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata: manifest.ManifestMetadata{
			Name: "demo-skill",
		},
		Spec: manifest.SkillSpec{
			Requires: manifest.SkillRequiresSpec{Bins: []string{"bash"}},
		},
	}
	if err := createSkillResourceDirs(skillRoot, skillManifest); err != nil {
		t.Fatal(err)
	}
	skillData, err := yaml.Marshal(skillManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "skill.manifest.yaml"), skillData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "relurpify_cfg", "agent.manifest.yaml"), []byte(`apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: testfu
spec:
  image: ghcr.io/example/runtime:latest
  runtime: gvisor
  defaults:
    permissions:
      filesystem:
        - action: fs:read
          path: /tmp/**
          justification: read
  agent:
    mode: primary
    model:
      provider: ollama
      name: qwen2.5-coder:14b
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newSkillDoctorCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, []string{"demo-skill"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "compatible") {
		t.Fatalf("unexpected binary-present output: %q", out.String())
	}
}

func TestSkillInitErrorBranches(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)

	blank := newSkillInitCmd()
	if err := blank.RunE(blank, []string{"   "}); err == nil || !strings.Contains(err.Error(), "skill name required") {
		t.Fatalf("expected blank-name error, got %v", err)
	}

	existingRoot := filepath.Join(ws, "relurpify_cfg", "skills", "already-there")
	if err := os.MkdirAll(existingRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := newSkillInitCmd()
	if err := existing.RunE(existing, []string{"already-there"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected exists error, got %v", err)
	}

	forceRoot := filepath.Join(ws, "relurpify_cfg", "skills", "force-fail")
	if err := os.MkdirAll(forceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(forceRoot, "scripts"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	forceCmd := newSkillInitCmd()
	if err := forceCmd.Flags().Set("force", "true"); err != nil {
		t.Fatal(err)
	}
	if err := forceCmd.RunE(forceCmd, []string{"force-fail"}); err == nil {
		t.Fatal("expected createSkillResourceDirs failure")
	}
}

func TestSkillInitWithTestsDefaultsAgent(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	root := filepath.Join(ws, "relurpify_cfg", "skills", "scaffold")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := newSkillInitCmd()
	if err := cmd.Flags().Set("with-tests", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("force", "true"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, []string{"scaffold"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "testsuite.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "agent_name: coding") {
		t.Fatalf("expected default agent in testsuite, got %s", string(data))
	}
}

func TestConfigSetWriteError(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	blocker := filepath.Join(ws, "relurpify_cfg")
	if err := os.WriteFile(blocker, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgFile = filepath.Join(blocker, "manifest.yaml")
	cmd := newConfigSetCmd()
	if err := cmd.RunE(cmd, []string{"foo", "bar"}); err == nil {
		t.Fatal("expected writeConfigMap failure")
	}
}
