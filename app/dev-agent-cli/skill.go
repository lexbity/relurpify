package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/lexcodex/relurpify/agents"
	appruntime "github.com/lexcodex/relurpify/app/relurpish/runtime"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	contractpkg "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/policybundle"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/framework/templates"
	"github.com/lexcodex/relurpify/testsuite/agenttest"
)

func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage skill packages",
	}
	cmd.AddCommand(
		newSkillInitCmd(),
		newSkillValidateCmd(),
		newSkillDoctorCmd(),
		newSkillTestCmd(),
	)
	return cmd
}

func newSkillInitCmd() *cobra.Command {
	var description string
	var version string
	var withTests bool
	var force bool
	var agentName string

	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Create a new skill scaffold",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			name := strings.TrimSpace(args[0])
			if name == "" {
				return fmt.Errorf("skill name required")
			}
			root := agents.SkillRoot(ws, name)
			if _, err := os.Stat(root); err == nil && !force {
				return fmt.Errorf("skill %s already exists (use --force to overwrite)", name)
			}
			if err := os.MkdirAll(root, 0o755); err != nil {
				return err
			}

			templatePath, err := templates.NewResolver().ResolveSkillManifestTemplate()
			if err != nil {
				return fmt.Errorf("resolve skill template: %w", err)
			}
			data, err := os.ReadFile(templatePath)
			if err != nil {
				return err
			}
			var skill manifest.SkillManifest
			if err := yaml.Unmarshal(data, &skill); err != nil {
				return err
			}
			skill.Metadata.Name = name
			if description != "" {
				skill.Metadata.Description = description
			}
			if version != "" {
				skill.Metadata.Version = version
			}
			encoded, err := yaml.Marshal(skill)
			if err != nil {
				return err
			}
			manifestPath := agents.SkillManifestPath(ws, name)
			if err := os.WriteFile(manifestPath, encoded, 0o644); err != nil {
				return err
			}

			if err := createSkillResourceDirs(root, skill); err != nil {
				return err
			}

			if withTests {
				if agentName == "" {
					agentName = "coding"
				}
				if err := writeSkillTestSuite(root, name, agentName, force); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Skill %s initialized at %s\n", name, manifestPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "Skill description")
	cmd.Flags().StringVar(&version, "version", "", "Skill version")
	cmd.Flags().BoolVar(&withTests, "with-tests", false, "Scaffold testsuite.yaml for the skill")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing files")
	cmd.Flags().StringVar(&agentName, "agent", "", "Agent name used in the generated testsuite.yaml")
	return cmd
}

func newSkillValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <name>",
		Short: "Validate a skill manifest and its resources",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			name := strings.TrimSpace(args[0])
			if name == "" {
				return fmt.Errorf("skill name required")
			}
			manifestPath := agents.SkillManifestPath(ws, name)
			skill, err := manifest.LoadSkillManifest(manifestPath)
			if err != nil {
				return err
			}
			paths := agents.ResolveSkillPaths(skill)
			if err := agents.ValidateSkillPaths(paths); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Skill %s valid\n", skill.Metadata.Name)
			return nil
		},
	}
	return cmd
}

func newSkillDoctorCmd() *cobra.Command {
	var agentName string
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "doctor <name>",
		Short: "Diagnose skill compatibility with tools and permissions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			name := strings.TrimSpace(args[0])
			if name == "" {
				return fmt.Errorf("skill name required")
			}
			skillPath := agents.SkillManifestPath(ws, name)
			skill, err := manifest.LoadSkillManifest(skillPath)
			if err != nil {
				return err
			}
			paths := agents.ResolveSkillPaths(skill)
			if err := agents.ValidateSkillPaths(paths); err != nil {
				return err
			}

			var agentManifest *manifest.AgentManifest
			if manifestPath != "" {
				agentManifest, err = manifest.LoadAgentManifest(manifestPath)
				if err != nil {
					return err
				}
			} else if agentName != "" {
				reg, err := buildRegistry(ws)
				if err != nil {
					return err
				}
				entry, ok := reg.Get(agentName)
				if !ok {
					return fmt.Errorf("agent %s not found", agentName)
				}
				agentManifest = entry
				manifestPath = entry.SourcePath
			}

			var (
				permissions       *fauthorization.PermissionManager
				effectiveContract *contractpkg.EffectiveAgentContract
			)
			if agentManifest != nil {
				effectiveContract, err = contractpkg.ResolveEffectiveAgentContract(ws, agentManifest, contractpkg.ResolveOptions{})
				if err != nil {
					return err
				}
				agentManifest.Spec.Permissions = effectiveContract.Permissions
				permissions, err = fauthorization.NewPermissionManager(ws, &agentManifest.Spec.Permissions, nil, nil)
				if err != nil {
					return err
				}
			}
			runner := fsandbox.NewLocalCommandRunner(ws, nil)
			capabilities, err := appruntime.BuildBuiltinCapabilityBundle(ws, runner, appruntime.CapabilityRegistryOptions{
				AgentID:           "skill-doctor",
				PermissionManager: permissions,
				AgentSpec:         effectiveAgentSpec(agentManifest, effectiveContract),
			})
			if err != nil {
				return err
			}
			registry := capabilities.Registry
			if effectiveContract != nil {
				compiledPolicy, err := policybundle.BuildFromContract(effectiveContract, permissions)
				if err != nil {
					return err
				}
				registry.SetPolicyEngine(compiledPolicy.Engine)
			}
			_ = registry // registry available for future checks
			for _, bin := range skill.Spec.Requires.Bins {
				bin = strings.TrimSpace(bin)
				if bin == "" {
					continue
				}
				if _, err := exec.LookPath(bin); err != nil {
					return fmt.Errorf("skill %s requires binary %q which was not found in PATH", skill.Metadata.Name, bin)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Skill %s compatible (manifest=%s)\n", skill.Metadata.Name, manifestPathOrDefault(manifestPath))
			return nil
		},
	}
	cmd.Flags().StringVar(&agentName, "agent", "", "Agent name from manifest registry")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Agent manifest path")
	return cmd
}

func newSkillTestCmd() *cobra.Command {
	var outDir string
	var endpoint string
	var model string
	var timeout time.Duration
	var sandbox bool

	cmd := &cobra.Command{
		Use:   "test <name>",
		Short: "Run testsuite.yaml for a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			name := strings.TrimSpace(args[0])
			if name == "" {
				return fmt.Errorf("skill name required")
			}
			root := agents.SkillRoot(ws, name)
			suitePath := filepath.Join(root, "testsuite.yaml")
			if _, err := os.Stat(suitePath); err != nil {
				return fmt.Errorf("testsuite.yaml missing for skill %s", name)
			}
			suite, err := agenttest.LoadSuite(suitePath)
			if err != nil {
				return err
			}

			manifestAbs := suite.ResolvePath(suite.Spec.Manifest)
			agentManifest, err := manifest.LoadAgentManifest(manifestAbs)
			if err != nil {
				return err
			}
			if !containsSkill(agentManifest.Spec.Skills, name) {
				agentManifest.Spec.Skills = append(agentManifest.Spec.Skills, name)
			}
			manifestBytes, err := yaml.Marshal(agentManifest)
			if err != nil {
				return err
			}
			derivedManifestPath := filepath.Join(root, "testsuite.manifest.yaml")
			if err := os.WriteFile(derivedManifestPath, manifestBytes, 0o644); err != nil {
				return err
			}
			suite.Spec.Manifest = derivedManifestPath

			runner := &agenttest.Runner{}
			opts := agenttest.RunOptions{
				TargetWorkspace:  ws,
				OutputDir:        outDir,
				Timeout:          timeout,
				Sandbox:          sandbox,
				ModelOverride:    model,
				EndpointOverride: endpoint,
			}
			_, err = runner.RunSuite(context.Background(), suite, opts)
			return err
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Override Ollama endpoint for this run")
	cmd.Flags().StringVar(&model, "model", "", "Override Ollama model name for this run")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Timeout per case (e.g. 45s)")
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory for run artifacts")
	cmd.Flags().BoolVar(&sandbox, "sandbox", false, "Run agenttest with sandboxed runtime")
	return cmd
}

func createSkillResourceDirs(root string, skill manifest.SkillManifest) error {
	entries := []string{}
	if len(skill.Spec.ResourcePaths.Scripts) == 0 {
		entries = append(entries, "scripts")
	} else {
		entries = append(entries, skill.Spec.ResourcePaths.Scripts...)
	}
	if len(skill.Spec.ResourcePaths.Resources) == 0 {
		entries = append(entries, "resources")
	} else {
		entries = append(entries, skill.Spec.ResourcePaths.Resources...)
	}
	if len(skill.Spec.ResourcePaths.Templates) == 0 {
		entries = append(entries, "templates")
	} else {
		entries = append(entries, skill.Spec.ResourcePaths.Templates...)
	}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		path := entry
		if !filepath.IsAbs(entry) {
			path = filepath.Join(root, entry)
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func writeSkillTestSuite(root, name, agentName string, force bool) error {
	path := filepath.Join(root, "testsuite.yaml")
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("testsuite.yaml already exists (use --force to overwrite)")
	}
	suite := agenttest.Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata: agenttest.SuiteMeta{
			Name:        fmt.Sprintf("%s-skill", name),
			Description: fmt.Sprintf("Skill tests for %s", name),
		},
		Spec: agenttest.SuiteSpec{
			AgentName: agentName,
			Manifest:  filepath.ToSlash(filepath.Join("..", "..", "agent.manifest.yaml")),
			Workspace: agenttest.WorkspaceSpec{
				Strategy:        "derived",
				TemplateProfile: "default",
			},
			Cases: []agenttest.CaseSpec{
				{
					Name:     "skill-smoke",
					TaskType: "analysis",
					Prompt:   fmt.Sprintf("Use the %s skill to summarize the expected workflow.", name),
					Expect: agenttest.ExpectSpec{
						MustSucceed: true,
					},
				},
			},
		},
	}
	data, err := yaml.Marshal(suite)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func effectiveAgentSpec(m *manifest.AgentManifest, contract *contractpkg.EffectiveAgentContract) *core.AgentRuntimeSpec {
	if contract != nil {
		return contract.AgentSpec
	}
	if m == nil {
		return nil
	}
	return m.Spec.Agent
}

func containsSkill(skills []string, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, s := range skills {
		if strings.EqualFold(strings.TrimSpace(s), name) {
			return true
		}
	}
	return false
}

func manifestPathOrDefault(path string) string {
	if path == "" {
		return "default"
	}
	return path
}
