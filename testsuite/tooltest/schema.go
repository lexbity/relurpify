// Package tooltest provides a YAML-driven test harness for manifest-defined
// tools. Each .tooltest.yaml file declares a tool invocation and expected
// outcome; the harness runs it through the real subprocess executor path
// with a stub CommandRunner.
package tooltest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ToolTestCase describes a single tool test loaded from .tooltest.yaml.
type ToolTestCase struct {
	// Path is the file path of the .tooltest.yaml.
	Path string `yaml:"-"`

	// Tool is the manifest tool name to invoke.
	Tool string `yaml:"tool"`

	// Args are the invocation arguments (map[string]interface{}).
	Args map[string]any `yaml:"args,omitempty"`

	// Stdin is optional standard input for the tool.
	Stdin string `yaml:"stdin,omitempty"`

	// Stdout is the canned stdout the stub runner returns.
	Stdout string `yaml:"stdout,omitempty"`

	// Stderr is the canned stderr the stub runner returns.
	Stderr string `yaml:"stderr,omitempty"`

	// ExitCode is the canned exit code the stub runner returns (default 0).
	ExitCode int `yaml:"exit_code,omitempty"`

	// Expect describes the expected outcome.
	Expect ToolTestExpect `yaml:"expect"`
}

// ToolTestExpect describes expected test outcomes.
type ToolTestExpect struct {
	// ExitCode expected (default 0).
	ExitCode int `yaml:"exit_code,omitempty"`

	// Success expected (default true).
	Success *bool `yaml:"success,omitempty"`

	// StdoutContains substrings that must appear in stdout.
	StdoutContains []string `yaml:"stdout_contains,omitempty"`

	// Stderr exact match (optional).
	Stderr string `yaml:"stderr,omitempty"`

	// StdoutJSON asserts stdout parses as JSON with optional field checks.
	StdoutJSON *ToolTestJSONExpect `yaml:"stdout_json,omitempty"`

	// ErrorContains substrings that must appear in the error message.
	ErrorContains []string `yaml:"error_contains,omitempty"`

	// HasStdoutRef asserts that stdout_ref is non-empty.
	HasStdoutRef bool `yaml:"has_stdout_ref,omitempty"`

	// HasStderrRef asserts that stderr_ref is non-empty.
	HasStderrRef bool `yaml:"has_stderr_ref,omitempty"`
}

// ToolTestJSONExpect describes expectations on parsed JSON output.
type ToolTestJSONExpect struct {
	// Type asserts the JSON type: "object", "array", "string", etc.
	Type string `yaml:"type,omitempty"`
}

// DefaultToolTestDir returns the canonical tooltest directory.
func DefaultToolTestDir(workspace string) string {
	return filepath.Join(workspace, "relurpify_cfg", "tooltests")
}

// LoadToolTests reads all .tooltest.yaml files from a directory.
func LoadToolTests(dir string) ([]*ToolTestCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*ToolTestCase
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".tooltest.yaml") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		tc, err := LoadToolTest(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		tc.Path = path
		out = append(out, tc)
	}
	return out, nil
}

// LoadToolTest reads a single .tooltest.yaml file.
func LoadToolTest(path string) (*ToolTestCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tc ToolTestCase
	if err := yaml.Unmarshal(data, &tc); err != nil {
		return nil, fmt.Errorf("yaml decode: %w", err)
	}
	if tc.Tool == "" {
		return nil, fmt.Errorf("tool name required")
	}
	return &tc, nil
}

// NormalizeForComparison cleans whitespace for assertion comparisons.
func NormalizeForComparison(s string) string {
	return strings.TrimSpace(s)
}
