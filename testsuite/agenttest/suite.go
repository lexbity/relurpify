package agenttest

import (
	"fmt"
	"os"
	"path/filepath"

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
	Models    []ModelSpec   `yaml:"models,omitempty"`
	Recording RecordingSpec `yaml:"recording,omitempty"`
	Cases     []CaseSpec    `yaml:"cases"`
}

type WorkspaceSpec struct {
	Strategy string   `yaml:"strategy,omitempty"` // copy|in_place
	Exclude  []string `yaml:"exclude,omitempty"`
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
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	TaskType    string            `yaml:"task_type,omitempty"`
	Prompt      string            `yaml:"prompt"`
	Context     map[string]any    `yaml:"context,omitempty"`
	Metadata    map[string]string `yaml:"metadata,omitempty"`
	Setup       SetupSpec         `yaml:"setup,omitempty"`
	Requires    RequiresSpec      `yaml:"requires,omitempty"`
	Expect      ExpectSpec        `yaml:"expect,omitempty"`
	Overrides   CaseOverrideSpec  `yaml:"overrides,omitempty"`
	Tags        []string          `yaml:"tags,omitempty"`
}

type RequiresSpec struct {
	Executables []string `yaml:"executables,omitempty"`
	Tools       []string `yaml:"tools,omitempty"`
}

type SetupSpec struct {
	Files   []SetupFileSpec `yaml:"files,omitempty"`
	GitInit bool            `yaml:"git_init,omitempty"`
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
}

type CaseOverrideSpec struct {
	MaxIterations int               `yaml:"max_iterations,omitempty"`
	Model         *ModelSpec        `yaml:"model,omitempty"`
	Recording     *RecordingSpec    `yaml:"recording,omitempty"`
	Workspace     *WorkspaceSpec    `yaml:"workspace,omitempty"`
	ExtraEnv      map[string]string `yaml:"extra_env,omitempty"`
	ToolMatrix    map[string]bool   `yaml:"tool_matrix,omitempty"`
	AllowedTools  []string          `yaml:"allowed_tools,omitempty"`
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
