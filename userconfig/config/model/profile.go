package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ModelProfileConfig is the canonical model profile definition loaded from relurpify_cfg/model/profiles.
type ModelProfileConfig struct {
	Schema      string `yaml:"schema"`
	Pattern     string `yaml:"pattern"`
	ToolCalling struct {
		Intent             string `yaml:"intent"`
		MaxConcurrentTools int    `yaml:"max_concurrent_tools"`
		DoubleEncodeArgs   bool   `yaml:"double_encode_args"`
	} `yaml:"tool_calling"`
	Context struct {
		MaxTokens             int `yaml:"max_tokens"`
		ResponseReserveTokens int `yaml:"response_reserve_tokens"`
	} `yaml:"context"`
	Generation struct {
		Temperature float64 `yaml:"temperature"`
		TopP        float64 `yaml:"top_p"`
	} `yaml:"generation"`
	SourcePath string `yaml:"-"`
}

// LoadProfileDir loads all *.llm.yaml files from model/profiles/.
// It returns a hard error if a blocking diagnostic is encountered.
func LoadProfileDir(dir string, decode Decoder) ([]*ModelProfileConfig, error) {
	loaded, diags, err := LoadProfileDirDetailed(dir, decode)
	if err != nil {
		return nil, err
	}
	if HasBlockingDiagnostics(diags) {
		return nil, diagnosticsError("profile", diags)
	}
	return loaded, nil
}

// LoadProfileDirDetailed loads all *.llm.yaml files and preserves partial
// results together with per-file diagnostics.
func LoadProfileDirDetailed(dir string, decode Decoder) ([]*ModelProfileConfig, []LoadDiagnostic, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil, fmt.Errorf("profile dir required")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve profile dir: %w", err)
	}
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, []LoadDiagnostic{{Path: absDir, Severity: "blocking", Message: fmt.Sprintf("read profile dir: %v", err)}}, nil
	}
	if len(entries) == 0 {
		return nil, []LoadDiagnostic{{Path: absDir, Severity: "blocking", Message: fmt.Sprintf("profile dir %q is empty", absDir)}}, nil
	}
	workspaceRoot := filepath.Clean(filepath.Join(absDir, "..", "..", ".."))

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".llm.yaml") {
			continue
		}
		paths = append(paths, filepath.Join(absDir, name))
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, []LoadDiagnostic{{Path: absDir, Severity: "blocking", Message: fmt.Sprintf("profile dir %q contains no *.llm.yaml files", absDir)}}, nil
	}

	var diags []LoadDiagnostic
	var defaultProfile *ModelProfileConfig
	out := make([]*ModelProfileConfig, 0, len(paths))
	for _, path := range paths {
		body, err := readConfigFile(workspaceRoot, path)
		if err != nil {
			diags = append(diags, LoadDiagnostic{Path: path, Severity: "blocking", Message: fmt.Sprintf("read %s: %v", path, err)})
			continue
		}
		if decode == nil {
			diags = append(diags, LoadDiagnostic{Path: path, Severity: "blocking", Message: fmt.Sprintf("decoder required for %s", path)})
			continue
		}
		var profile ModelProfileConfig
		if _, err := decode(path, body, &profile); err != nil {
			severity := "blocking"
			if !isDefaultProfilePath(path) {
				severity = "warning"
			}
			diags = append(diags, LoadDiagnostic{Path: path, Severity: severity, Message: err.Error()})
			continue
		}
		profile.SourcePath = path
		if err := profile.Validate(); err != nil {
			severity := "blocking"
			if !isDefaultProfilePath(path) {
				severity = "warning"
			}
			diags = append(diags, LoadDiagnostic{Path: path, Severity: severity, Message: fmt.Sprintf("%s: %v", path, err)})
			continue
		}
		if isDefaultProfilePath(path) {
			defaultProfile = &profile
			continue
		}
		out = append(out, &profile)
	}
	if defaultProfile == nil {
		diags = append(diags, LoadDiagnostic{Path: filepath.Join(absDir, "default.llm.yaml"), Severity: "blocking", Message: fmt.Sprintf("default.llm.yaml required in %s", absDir)})
	}
	out = append(out, defaultProfile)
	return out, diags, nil
}

// MatchProfile returns the first matching profile by glob, falling back to default.llm.yaml.
func MatchProfile(profiles []*ModelProfileConfig, modelName string) *ModelProfileConfig {
	modelName = strings.TrimSpace(modelName)
	if len(profiles) == 0 {
		return nil
	}
	var defaultProfile *ModelProfileConfig
	for _, profile := range profiles {
		if profile == nil {
			continue
		}
		if isDefaultProfilePath(profile.SourcePath) {
			defaultProfile = profile
			continue
		}
		pattern := strings.TrimSpace(profile.Pattern)
		if pattern == "" {
			continue
		}
		ok, err := filepath.Match(pattern, modelName)
		if err != nil {
			continue
		}
		if ok {
			return profile
		}
	}
	return defaultProfile
}

func (p ModelProfileConfig) Validate() error {
	if strings.TrimSpace(p.Pattern) == "" {
		return fmt.Errorf("pattern required")
	}
	if _, err := filepath.Match(p.Pattern, "model"); err != nil {
		return fmt.Errorf("pattern invalid: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(p.ToolCalling.Intent)) {
	case "native", "prompt_based", "auto":
	default:
		return fmt.Errorf("tool_calling.intent invalid")
	}
	if p.ToolCalling.MaxConcurrentTools < 0 || p.ToolCalling.MaxConcurrentTools > 32 {
		return fmt.Errorf("tool_calling.max_concurrent_tools must be in [0,32]")
	}
	if p.Context.MaxTokens <= 0 {
		return fmt.Errorf("context.max_tokens must be positive")
	}
	if p.Context.ResponseReserveTokens < 0 {
		return fmt.Errorf("context.response_reserve_tokens must be >= 0")
	}
	if p.Generation.Temperature < 0 || p.Generation.Temperature > 2 {
		return fmt.Errorf("generation.temperature must be in [0,2]")
	}
	if p.Generation.TopP < 0 || p.Generation.TopP > 1 {
		return fmt.Errorf("generation.top_p must be in [0,1]")
	}
	if isDefaultProfilePath(p.SourcePath) && strings.TrimSpace(p.Pattern) != "*" {
		return fmt.Errorf("default.llm.yaml must use pattern \"*\"")
	}
	return nil
}

func isDefaultProfilePath(path string) bool {
	return strings.EqualFold(filepath.Base(path), "default.llm.yaml")
}

func diagnosticsError(section string, diags []LoadDiagnostic) error {
	if len(diags) == 0 {
		return nil
	}
	var errs []error
	for _, diag := range diags {
		errs = append(errs, fmt.Errorf("%s: %s", diag.Path, diag.Message))
	}
	return fmt.Errorf("load %s: %w", section, errors.Join(errs...))
}
