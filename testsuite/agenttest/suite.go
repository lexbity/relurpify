package agenttest

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lexcodex/relurpify/framework/core"
	"gopkg.in/yaml.v3"
)

type Suite struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   SuiteMeta `yaml:"metadata"`
	Spec       SuiteSpec `yaml:"spec"`
	SourcePath string    `yaml:"-"`
}

type SuiteMeta struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

type SuiteSpec struct {
	AgentName string `yaml:"agent_name"`
	Manifest  string `yaml:"manifest"`

	Workspace WorkspaceSpec `yaml:"workspace"`
	Memory    MemorySpec    `yaml:"memory,omitempty"`
	Models    []ModelSpec   `yaml:"models,omitempty"`
	Recording RecordingSpec `yaml:"recording,omitempty"`
	Cases     []CaseSpec    `yaml:"cases"`
}

type WorkspaceSpec struct {
	Strategy        string          `yaml:"strategy,omitempty"` // derived
	TemplateProfile string          `yaml:"template_profile,omitempty"`
	Exclude         []string        `yaml:"exclude,omitempty"`
	IgnoreChanges   []string        `yaml:"ignore_changes,omitempty"`
	Files           []SetupFileSpec `yaml:"files,omitempty"`
}

type ModelSpec struct {
	Name     string `yaml:"name"`
	Endpoint string `yaml:"endpoint,omitempty"`
}

type RecordingSpec struct {
	Mode string `yaml:"mode,omitempty"` // off|record|replay
	Tape string `yaml:"tape,omitempty"`
}

type CaseSpec struct {
	Name            string                        `yaml:"name"`
	Description     string                        `yaml:"description,omitempty"`
	TaskType        string                        `yaml:"task_type,omitempty"`
	Prompt          string                        `yaml:"prompt"`
	Context         map[string]any                `yaml:"context,omitempty"`
	Metadata        map[string]string             `yaml:"metadata,omitempty"`
	BrowserFixtures map[string]BrowserFixtureSpec `yaml:"browser_fixtures,omitempty"`
	Setup           SetupSpec                     `yaml:"setup,omitempty"`
	Requires        RequiresSpec                  `yaml:"requires,omitempty"`
	Expect          ExpectSpec                    `yaml:"expect,omitempty"`
	Overrides       CaseOverrideSpec              `yaml:"overrides,omitempty"`
	Tags            []string                      `yaml:"tags,omitempty"`
}

type BrowserFixtureSpec struct {
	Path        string            `yaml:"path,omitempty"`
	File        string            `yaml:"file,omitempty"`
	Content     string            `yaml:"content,omitempty"`
	ContentType string            `yaml:"content_type,omitempty"`
	Status      int               `yaml:"status,omitempty"`
	Headers     map[string]string `yaml:"headers,omitempty"`
}

type RequiresSpec struct {
	Executables []string `yaml:"executables,omitempty"`
	Tools       []string `yaml:"tools,omitempty"`
}

type SetupSpec struct {
	Files     []SetupFileSpec    `yaml:"files,omitempty"`
	GitInit   bool               `yaml:"git_init,omitempty"`
	Memory    MemorySeedSpec     `yaml:"memory,omitempty"`
	Workflows []WorkflowSeedSpec `yaml:"workflows,omitempty"`
}

type SetupFileSpec struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content"`
	Mode    string `yaml:"mode,omitempty"`
}

type ExpectSpec struct {
	MustSucceed bool `yaml:"must_succeed,omitempty"`

	OutputContains []string `yaml:"output_contains,omitempty"`
	OutputRegex    []string `yaml:"output_regex,omitempty"`

	NoFileChanges bool     `yaml:"no_file_changes,omitempty"`
	FilesChanged  []string `yaml:"files_changed,omitempty"`

	ToolCallsMustInclude []string `yaml:"tool_calls_must_include,omitempty"`
	ToolCallsMustExclude []string `yaml:"tool_calls_must_exclude,omitempty"`
	MaxToolCalls         int      `yaml:"max_tool_calls,omitempty"`
	StateKeysMustExist   []string `yaml:"state_keys_must_exist,omitempty"`
}

type CaseOverrideSpec struct {
	MaxIterations        int                       `yaml:"max_iterations,omitempty"`
	Model                *ModelSpec                `yaml:"model,omitempty"`
	Recording            *RecordingSpec            `yaml:"recording,omitempty"`
	Workspace            *WorkspaceSpec            `yaml:"workspace,omitempty"`
	Memory               *MemorySpec               `yaml:"memory,omitempty"`
	ExtraEnv             map[string]string         `yaml:"extra_env,omitempty"`
	AllowedCapabilities  []core.CapabilitySelector `yaml:"allowed_capabilities,omitempty"`
	RestrictCapabilities bool                      `yaml:"restrict_capabilities,omitempty"`
	ControlFlow          string                    `yaml:"control_flow,omitempty"`
}

type MemorySpec struct {
	Backend   string              `yaml:"backend,omitempty"` // hybrid|sqlite_runtime
	Retrieval MemoryRetrievalSpec `yaml:"retrieval,omitempty"`
}

type MemoryRetrievalSpec struct {
	Embedder string `yaml:"embedder,omitempty"` // "", "test"
}

type MemorySeedSpec struct {
	Declarative []DeclarativeMemorySeedSpec `yaml:"declarative,omitempty"`
	Procedural  []ProceduralMemorySeedSpec  `yaml:"procedural,omitempty"`
}

type DeclarativeMemorySeedSpec struct {
	RecordID    string         `yaml:"record_id"`
	Scope       string         `yaml:"scope,omitempty"`
	Kind        string         `yaml:"kind,omitempty"`
	Title       string         `yaml:"title,omitempty"`
	Content     string         `yaml:"content,omitempty"`
	Summary     string         `yaml:"summary,omitempty"`
	WorkflowID  string         `yaml:"workflow_id,omitempty"`
	TaskID      string         `yaml:"task_id,omitempty"`
	ProjectID   string         `yaml:"project_id,omitempty"`
	ArtifactRef string         `yaml:"artifact_ref,omitempty"`
	Tags        []string       `yaml:"tags,omitempty"`
	Metadata    map[string]any `yaml:"metadata,omitempty"`
	Verified    bool           `yaml:"verified,omitempty"`
}

type ProceduralMemorySeedSpec struct {
	RoutineID              string                    `yaml:"routine_id"`
	Scope                  string                    `yaml:"scope,omitempty"`
	Kind                   string                    `yaml:"kind,omitempty"`
	Name                   string                    `yaml:"name,omitempty"`
	Description            string                    `yaml:"description,omitempty"`
	Summary                string                    `yaml:"summary,omitempty"`
	WorkflowID             string                    `yaml:"workflow_id,omitempty"`
	TaskID                 string                    `yaml:"task_id,omitempty"`
	ProjectID              string                    `yaml:"project_id,omitempty"`
	BodyRef                string                    `yaml:"body_ref,omitempty"`
	InlineBody             string                    `yaml:"inline_body,omitempty"`
	CapabilityDependencies []core.CapabilitySelector `yaml:"capability_dependencies,omitempty"`
	VerificationMetadata   map[string]any            `yaml:"verification_metadata,omitempty"`
	PolicySnapshotID       string                    `yaml:"policy_snapshot_id,omitempty"`
	Verified               bool                      `yaml:"verified,omitempty"`
	Version                int                       `yaml:"version,omitempty"`
	ReuseCount             int                       `yaml:"reuse_count,omitempty"`
}

type WorkflowSeedSpec struct {
	Workflow  WorkflowRecordSeedSpec      `yaml:"workflow"`
	Runs      []WorkflowRunSeedSpec       `yaml:"runs,omitempty"`
	Knowledge []WorkflowKnowledgeSeedSpec `yaml:"knowledge,omitempty"`
}

type WorkflowRecordSeedSpec struct {
	WorkflowID   string         `yaml:"workflow_id"`
	TaskID       string         `yaml:"task_id,omitempty"`
	TaskType     string         `yaml:"task_type,omitempty"`
	Instruction  string         `yaml:"instruction,omitempty"`
	Status       string         `yaml:"status,omitempty"`
	CursorStepID string         `yaml:"cursor_step_id,omitempty"`
	Metadata     map[string]any `yaml:"metadata,omitempty"`
}

type WorkflowRunSeedSpec struct {
	RunID          string         `yaml:"run_id"`
	WorkflowID     string         `yaml:"workflow_id,omitempty"`
	Status         string         `yaml:"status,omitempty"`
	AgentName      string         `yaml:"agent_name,omitempty"`
	AgentMode      string         `yaml:"agent_mode,omitempty"`
	RuntimeVersion string         `yaml:"runtime_version,omitempty"`
	Metadata       map[string]any `yaml:"metadata,omitempty"`
}

type WorkflowKnowledgeSeedSpec struct {
	RecordID   string         `yaml:"record_id"`
	WorkflowID string         `yaml:"workflow_id,omitempty"`
	StepRunID  string         `yaml:"step_run_id,omitempty"`
	StepID     string         `yaml:"step_id,omitempty"`
	Kind       string         `yaml:"kind,omitempty"`
	Title      string         `yaml:"title,omitempty"`
	Content    string         `yaml:"content,omitempty"`
	Status     string         `yaml:"status,omitempty"`
	Metadata   map[string]any `yaml:"metadata,omitempty"`
}

func LoadSuite(path string) (*Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var suite Suite
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return nil, err
	}
	suite.SourcePath = path
	if err := suite.Validate(); err != nil {
		return nil, err
	}
	return &suite, nil
}

func (s *Suite) Validate() error {
	if s.APIVersion == "" {
		return fmt.Errorf("suite missing apiVersion")
	}
	if s.Kind == "" {
		return fmt.Errorf("suite missing kind")
	}
	if s.Spec.AgentName == "" {
		return fmt.Errorf("suite missing spec.agent_name")
	}
	if s.Spec.Manifest == "" {
		return fmt.Errorf("suite missing spec.manifest")
	}
	strategy := s.Spec.Workspace.Strategy
	if strategy == "" {
		s.Spec.Workspace.Strategy = "derived"
	} else if strategy != "derived" {
		return fmt.Errorf("suite workspace.strategy %q unsupported; use derived", strategy)
	}
	if s.Spec.Workspace.TemplateProfile == "" {
		s.Spec.Workspace.TemplateProfile = "default"
	}
	if err := validateMemorySpec(s.Spec.Memory, "suite spec.memory"); err != nil {
		return err
	}
	if len(s.Spec.Cases) == 0 {
		return fmt.Errorf("suite missing spec.cases")
	}
	for i, c := range s.Spec.Cases {
		if c.Name == "" {
			return fmt.Errorf("suite case[%d] missing name", i)
		}
		if c.Prompt == "" {
			return fmt.Errorf("suite case[%s] missing prompt", c.Name)
		}
		if c.Overrides.Memory != nil {
			if err := validateMemorySpec(*c.Overrides.Memory, fmt.Sprintf("suite case[%s] overrides.memory", c.Name)); err != nil {
				return err
			}
		}
		for fixtureName, fixture := range c.BrowserFixtures {
			if fixtureName == "" {
				return fmt.Errorf("suite case[%s] has browser fixture with empty name", c.Name)
			}
			if fixture.File == "" && fixture.Content == "" {
				return fmt.Errorf("suite case[%s] browser fixture[%s] missing file or content", c.Name, fixtureName)
			}
		}
		if err := validateSetup(c.Setup, c.Name); err != nil {
			return err
		}
	}
	return nil
}

func validateMemorySpec(spec MemorySpec, location string) error {
	switch spec.Backend {
	case "", "hybrid", "sqlite_runtime":
	default:
		return fmt.Errorf("%s backend %q unsupported", location, spec.Backend)
	}
	switch spec.Retrieval.Embedder {
	case "", "test":
	default:
		return fmt.Errorf("%s retrieval.embedder %q unsupported", location, spec.Retrieval.Embedder)
	}
	return nil
}

func validateSetup(setup SetupSpec, caseName string) error {
	for _, record := range setup.Memory.Declarative {
		if record.RecordID == "" {
			return fmt.Errorf("suite case[%s] setup.memory.declarative missing record_id", caseName)
		}
	}
	for _, record := range setup.Memory.Procedural {
		if record.RoutineID == "" {
			return fmt.Errorf("suite case[%s] setup.memory.procedural missing routine_id", caseName)
		}
	}
	for _, workflow := range setup.Workflows {
		if workflow.Workflow.WorkflowID == "" {
			return fmt.Errorf("suite case[%s] setup.workflows missing workflow.workflow_id", caseName)
		}
		for _, run := range workflow.Runs {
			if run.RunID == "" {
				return fmt.Errorf("suite case[%s] setup.workflows[%s] run missing run_id", caseName, workflow.Workflow.WorkflowID)
			}
		}
		for _, knowledge := range workflow.Knowledge {
			if knowledge.RecordID == "" {
				return fmt.Errorf("suite case[%s] setup.workflows[%s] knowledge missing record_id", caseName, workflow.Workflow.WorkflowID)
			}
		}
	}
	return nil
}

func (s *Suite) ResolvePath(rel string) string {
	if rel == "" {
		return ""
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	base := "."
	if s != nil && s.SourcePath != "" {
		base = filepath.Dir(s.SourcePath)
	}
	return filepath.Clean(filepath.Join(base, rel))
}
