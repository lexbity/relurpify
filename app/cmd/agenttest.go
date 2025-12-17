package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/lexcodex/relurpify/agenttest"
	"github.com/lexcodex/relurpify/framework"
)

func newAgentTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agenttest",
		Short: "Run YAML-driven agent test suites",
	}
	cmd.AddCommand(newAgentTestInitCmd(), newAgentTestRunCmd())
	return cmd
}

func newAgentTestInitCmd() *cobra.Command {
	var agentList string
	var model string
	var endpoint string
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize relurpify_cfg manifests + testsuites",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			if model == "" {
				model = defaultModelName()
			}
			if endpoint == "" {
				endpoint = defaultEndpoint()
			}
			agentsToInit := splitCSV(agentList)
			if len(agentsToInit) == 0 {
				agentsToInit = []string{"coding", "planner", "react", "reflection", "expert", "eternal"}
			}
			cfgDir := filepath.Join(ws, "relurpify_cfg")
			agentsDir := filepath.Join(cfgDir, "agents")
			suitesDir := filepath.Join(cfgDir, "testsuites")
			if err := os.MkdirAll(agentsDir, 0o755); err != nil {
				return err
			}
			if err := os.MkdirAll(suitesDir, 0o755); err != nil {
				return err
			}
			wsGlob := filepath.ToSlash(filepath.Join(ws, "**"))
			manifestTemplate := baseManifestTemplate(wsGlob, model, endpoint)
			for _, name := range agentsToInit {
				manifest := manifestTemplate
				manifest.Metadata.Name = name
				manifest.Spec.Agent.Implementation = agentImplementation(name)
				manifest.Spec.Agent.Prompt = defaultAgentPrompt(name)
				manifest.Spec.Agent.Model.Name = model
				manifest.Spec.Permissions.Network = []framework.NetworkPermission{{
					Direction:   "egress",
					Protocol:    "tcp",
					Host:        "localhost",
					Port:        11434,
					Description: "Ollama",
				}}
				setToolMatrixForAgent(&manifest, name)
				if err := manifest.Validate(); err != nil {
					return fmt.Errorf("manifest %s invalid: %w", name, err)
				}
				path := filepath.Join(agentsDir, fmt.Sprintf("%s.yaml", sanitizeName(name)))
				if !force {
					if _, err := os.Stat(path); err == nil {
						return fmt.Errorf("refusing to overwrite %s (use --force)", path)
					}
				}
				data, err := yaml.Marshal(manifest)
				if err != nil {
					return err
				}
				if err := os.WriteFile(path, data, 0o644); err != nil {
					return err
				}

				suite := defaultSuite(ws, name, endpoint, model)
				suitePath := filepath.Join(suitesDir, fmt.Sprintf("%s.testsuite.yaml", sanitizeName(name)))
				if !force {
					if _, err := os.Stat(suitePath); err == nil {
						return fmt.Errorf("refusing to overwrite %s (use --force)", suitePath)
					}
				}
				suiteData, err := yaml.Marshal(suite)
				if err != nil {
					return err
				}
				if err := os.WriteFile(suitePath, suiteData, 0o644); err != nil {
					return err
				}
			}

			// Language-focused coding suites + manifests.
			if containsString(agentsToInit, "coding") {
				codingManifestPath := filepath.Join(agentsDir, "coding.yaml")
				if fileExists(codingManifestPath) {
					// Go-specific (adds tool access already covered by base manifest).
					_ = writeDerivedManifest(filepath.Join(agentsDir, "coding-go.yaml"), codingManifestPath, "coding-go", "Go-focused coding agent manifest", nil, force)
					_ = writeLanguageSuite(filepath.Join(suitesDir, "coding.go.testsuite.yaml"), "coding.go", "../agents/coding-go.yaml", "Go-focused coding prompts", model, endpoint, force)

					// Rust-specific (needs cargo executable permission).
					execAdd := []framework.ExecutablePermission{{Binary: "cargo", Args: []string{"*"}}}
					_ = writeDerivedManifest(filepath.Join(agentsDir, "coding-rust.yaml"), codingManifestPath, "coding-rust", "Rust-focused coding agent manifest", execAdd, force)
					_ = writeLanguageSuite(filepath.Join(suitesDir, "coding.rust.testsuite.yaml"), "coding.rust", "../agents/coding-rust.yaml", "Rust-focused coding prompts", model, endpoint, force)

					// Python-specific.
					execAdd = []framework.ExecutablePermission{{Binary: "python3", Args: []string{"*"}}}
					_ = writeDerivedManifest(filepath.Join(agentsDir, "coding-python.yaml"), codingManifestPath, "coding-python", "Python-focused coding agent manifest", execAdd, force)
					_ = writeLanguageSuite(filepath.Join(suitesDir, "coding.python.testsuite.yaml"), "coding.python", "../agents/coding-python.yaml", "Python-focused coding prompts", model, endpoint, force)

					// Node.js-specific.
					execAdd = []framework.ExecutablePermission{
						{Binary: "node", Args: []string{"*"}},
						{Binary: "npm", Args: []string{"*"}},
					}
					_ = writeDerivedManifest(filepath.Join(agentsDir, "coding-node.yaml"), codingManifestPath, "coding-node", "Node.js-focused coding agent manifest", execAdd, force)
					_ = writeLanguageSuite(filepath.Join(suitesDir, "coding.node.testsuite.yaml"), "coding.node", "../agents/coding-node.yaml", "Node.js-focused coding prompts", model, endpoint, force)

					// SQLite-specific.
					execAdd = []framework.ExecutablePermission{{Binary: "sqlite3", Args: []string{"*"}}}
					_ = writeDerivedManifest(filepath.Join(agentsDir, "coding-sqlite.yaml"), codingManifestPath, "coding-sqlite", "SQLite-focused coding agent manifest", execAdd, force)
					_ = writeLanguageSuite(filepath.Join(suitesDir, "coding.sqlite.testsuite.yaml"), "coding.sqlite", "../agents/coding-sqlite.yaml", "SQLite-focused coding prompts", model, endpoint, force)
				}
			}
			// Keep a default manifest at the conventional path for other CLIs.
			defaultManifest := filepath.Join(cfgDir, "agent.manifest.yaml")
			if force || !fileExists(defaultManifest) {
				codingPath := filepath.Join(agentsDir, "coding.yaml")
				if fileExists(codingPath) {
					data, _ := os.ReadFile(codingPath)
					_ = os.WriteFile(defaultManifest, data, 0o644)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Initialized %d manifests + testsuites in %s\n", len(agentsToInit), cfgDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&agentList, "agents", "", "Comma-separated agent names to init (default: builtin set)")
	cmd.Flags().StringVar(&model, "model", "", "Default Ollama model name")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Default Ollama endpoint")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing files")
	return cmd
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func writeDerivedManifest(outPath, basePath, name, description string, extraExec []framework.ExecutablePermission, force bool) error {
	if !force {
		if _, err := os.Stat(outPath); err == nil {
			return nil
		}
	}
	manifest, err := framework.LoadAgentManifest(basePath)
	if err != nil {
		return err
	}
	manifest.Metadata.Name = name
	manifest.Metadata.Description = description
	if len(extraExec) > 0 {
		manifest.Spec.Permissions.Executables = append(manifest.Spec.Permissions.Executables, extraExec...)
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, data, 0o644)
}

func writeLanguageSuite(outPath, suiteName, manifestRel, description, model, endpoint string, force bool) error {
	if !force {
		if _, err := os.Stat(outPath); err == nil {
			return nil
		}
	}
	suite := agenttest.Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata: agenttest.SuiteMeta{
			Name:        suiteName,
			Description: description,
		},
		Spec: agenttest.SuiteSpec{
			AgentName: "coding",
			Manifest:  manifestRel,
			Workspace: agenttest.WorkspaceSpec{
				Strategy: "in_place",
				Exclude: []string{
					".git/**",
					".gocache/**",
					".gomodcache/**",
					"relurpify_cfg/test_runs/**",
				},
			},
			Models: []agenttest.ModelSpec{{Name: model, Endpoint: endpoint}},
		},
	}
	switch suiteName {
	case "coding.go":
		suite.Spec.Cases = []agenttest.CaseSpec{
			{
				Name:     "easy_fix_bug_and_run_tests",
				TaskType: string(framework.TaskTypeCodeModification),
				Setup: agenttest.SetupSpec{Files: []agenttest.SetupFileSpec{
					{
						Path: "testsuite/agenttest_fixtures/gosuite/mathutil/mathutil.go",
						Content: `package mathutil

// Add returns the sum of a and b.
func Add(a, b int) int {
	return a + b + 1
}
`,
					},
					{
						Path: "testsuite/agenttest_fixtures/gosuite/mathutil/mathutil_test.go",
						Content: `package mathutil

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 2) != 4 {
		t.Fatalf("expected Add(2,2)=4")
	}
}
`,
					},
				}},
				Overrides: agenttest.CaseOverrideSpec{AllowedTools: []string{"cli_go"}},
				Prompt:    "Fix the failing tests in testsuite/agenttest_fixtures/gosuite/mathutil (do not change tests), then run cli_go args [\"test\",\"./testsuite/agenttest_fixtures/gosuite/mathutil\"] and confirm it passes.",
				Expect: agenttest.ExpectSpec{
					MustSucceed:          true,
					FilesChanged:         []string{"testsuite/agenttest_fixtures/gosuite/mathutil/mathutil.go"},
					ToolCallsMustInclude: []string{"cli_go"},
					ToolCallsMustExclude: []string{"exec_run_tests"},
				},
			},
			{
				Name:     "medium_add_mul_and_test",
				TaskType: string(framework.TaskTypeCodeModification),
				Setup: agenttest.SetupSpec{Files: []agenttest.SetupFileSpec{
					{
						Path: "testsuite/agenttest_fixtures/gosuite/mathutil/mathutil.go",
						Content: `package mathutil

// Add returns the sum of a and b.
func Add(a, b int) int {
	return a + b
}
`,
					},
					{
						Path: "testsuite/agenttest_fixtures/gosuite/mathutil/mul_test.go",
						Content: `package mathutil

import "testing"

func TestMul(t *testing.T) {
	if Mul(3, 4) != 12 {
		t.Fatalf("expected Mul(3,4)=12")
	}
}
`,
					},
				}},
				Overrides: agenttest.CaseOverrideSpec{AllowedTools: []string{"cli_go"}},
				Prompt:    "Implement Mul(a,b int) in testsuite/agenttest_fixtures/gosuite/mathutil/mathutil.go (do not change tests), then run cli_go args [\"test\",\"./testsuite/agenttest_fixtures/gosuite/mathutil\"] and confirm it passes.",
				Expect: agenttest.ExpectSpec{
					MustSucceed:          true,
					FilesChanged:         []string{"testsuite/agenttest_fixtures/gosuite/mathutil/mathutil.go"},
					ToolCallsMustInclude: []string{"cli_go"},
					ToolCallsMustExclude: []string{"exec_run_tests"},
				},
			},
			{
				Name:     "hard_add_sumall_and_test",
				TaskType: string(framework.TaskTypeCodeModification),
				Setup: agenttest.SetupSpec{Files: []agenttest.SetupFileSpec{
					{
						Path: "testsuite/agenttest_fixtures/gosuite/mathutil/mathutil.go",
						Content: `package mathutil

// Add returns the sum of a and b.
func Add(a, b int) int {
	return a + b
}
`,
					},
					{
						Path: "testsuite/agenttest_fixtures/gosuite/mathutil/sumall_test.go",
						Content: `package mathutil

import "testing"

func TestSumAll(t *testing.T) {
	if SumAll(nil) != 0 {
		t.Fatalf("expected SumAll(nil)=0")
	}
	if SumAll([]int{}) != 0 {
		t.Fatalf("expected SumAll([])=0")
	}
	if SumAll([]int{1, 2, 3}) != 6 {
		t.Fatalf("expected SumAll([1,2,3])=6")
	}
}
`,
					},
				}},
				Overrides: agenttest.CaseOverrideSpec{AllowedTools: []string{"cli_go"}},
				Prompt:    "Implement SumAll(nums []int) int in testsuite/agenttest_fixtures/gosuite/mathutil/mathutil.go (do not change tests), then run cli_go args [\"test\",\"./testsuite/agenttest_fixtures/gosuite/mathutil\"] and confirm it passes.",
				Expect: agenttest.ExpectSpec{
					MustSucceed:          true,
					ToolCallsMustInclude: []string{"cli_go"},
					ToolCallsMustExclude: []string{"exec_run_tests"},
				},
			},
		}
	case "coding.rust":
		suite.Spec.Cases = []agenttest.CaseSpec{
			{
				Name:     "easy_fix_bug_and_run_tests",
				TaskType: string(framework.TaskTypeCodeModification),
				Requires: agenttest.RequiresSpec{
					Executables: []string{"cargo"},
					Tools:       []string{"cli_cargo"},
				},
				Setup: agenttest.SetupSpec{Files: []agenttest.SetupFileSpec{{
					Path: "testsuite/agenttest_fixtures/rustsuite/src/lib.rs",
					Content: `pub fn add(a: i32, b: i32) -> i32 {
    a + b + 1
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_add() {
        assert_eq!(add(2, 2), 4);
    }
}
`,
				}}},
				Overrides: agenttest.CaseOverrideSpec{AllowedTools: []string{"cli_cargo", "cli_rustfmt"}},
				Prompt:    "Fix the failing tests in testsuite/agenttest_fixtures/rustsuite (do not change test expectations), then run cli_cargo args [\"test\"] with working_directory \"testsuite/agenttest_fixtures/rustsuite\".",
				Expect: agenttest.ExpectSpec{
					MustSucceed:          true,
					FilesChanged:         []string{"testsuite/agenttest_fixtures/rustsuite/src/lib.rs"},
					ToolCallsMustInclude: []string{"cli_cargo"},
					ToolCallsMustExclude: []string{"exec_run_tests"},
				},
			},
			{
				Name:     "medium_add_mul_and_test",
				TaskType: string(framework.TaskTypeCodeModification),
				Requires: agenttest.RequiresSpec{
					Executables: []string{"cargo"},
					Tools:       []string{"cli_cargo"},
				},
				Setup: agenttest.SetupSpec{Files: []agenttest.SetupFileSpec{{
					Path: "testsuite/agenttest_fixtures/rustsuite/src/lib.rs",
					Content: `pub fn add(a: i32, b: i32) -> i32 {
    a + b
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_add() {
        assert_eq!(add(2, 2), 4);
    }

    #[test]
    fn test_mul() {
        assert_eq!(mul(3, 4), 12);
    }
}
`,
				}}},
				Overrides: agenttest.CaseOverrideSpec{AllowedTools: []string{"cli_cargo", "cli_rustfmt"}},
				Prompt:    "Implement mul(a: i32, b: i32) -> i32 in testsuite/agenttest_fixtures/rustsuite/src/lib.rs (do not change tests), then run cli_cargo args [\"test\"] with working_directory \"testsuite/agenttest_fixtures/rustsuite\".",
				Expect: agenttest.ExpectSpec{
					MustSucceed:          true,
					FilesChanged:         []string{"testsuite/agenttest_fixtures/rustsuite/src/lib.rs"},
					ToolCallsMustInclude: []string{"cli_cargo"},
					ToolCallsMustExclude: []string{"exec_run_tests"},
				},
			},
		}
	case "coding.python":
		suite.Spec.Cases = []agenttest.CaseSpec{
			{
				Name:     "easy_fix_bug_and_run_tests",
				TaskType: string(framework.TaskTypeCodeModification),
				Requires: agenttest.RequiresSpec{
					Executables: []string{"python3"},
					Tools:       []string{"cli_python"},
				},
				Setup: agenttest.SetupSpec{Files: []agenttest.SetupFileSpec{
					{
						Path: "testsuite/agenttest_fixtures/pysuite/calc.py",
						Content: `def add(a: int, b: int) -> int:
    return a + b + 1


def mul(a: int, b: int) -> int:
    return a * b
`,
					},
					{
						Path: "testsuite/agenttest_fixtures/pysuite/test_calc.py",
						Content: `import unittest

import calc


class TestCalc(unittest.TestCase):
    def test_add(self) -> None:
        self.assertEqual(calc.add(2, 2), 4)

    def test_mul(self) -> None:
        self.assertEqual(calc.mul(3, 4), 12)


if __name__ == "__main__":
    unittest.main()
`,
					},
				}},
				Overrides: agenttest.CaseOverrideSpec{AllowedTools: []string{"cli_python"}},
				Prompt:    "Fix the failing tests in testsuite/agenttest_fixtures/pysuite (do not change test_calc.py), then run cli_python args [\"test_calc.py\"] with working_directory \"testsuite/agenttest_fixtures/pysuite\".",
				Expect: agenttest.ExpectSpec{
					MustSucceed:          true,
					FilesChanged:         []string{"testsuite/agenttest_fixtures/pysuite/calc.py"},
					ToolCallsMustInclude: []string{"cli_python"},
					ToolCallsMustExclude: []string{"exec_run_tests"},
				},
			},
			{
				Name:     "medium_add_div_and_test",
				TaskType: string(framework.TaskTypeCodeModification),
				Requires: agenttest.RequiresSpec{
					Executables: []string{"python3"},
					Tools:       []string{"cli_python"},
				},
				Setup: agenttest.SetupSpec{Files: []agenttest.SetupFileSpec{
					{
						Path: "testsuite/agenttest_fixtures/pysuite/calc.py",
						Content: `def add(a: int, b: int) -> int:
    return a + b


def mul(a: int, b: int) -> int:
    return a * b
`,
					},
					{
						Path: "testsuite/agenttest_fixtures/pysuite/test_calc.py",
						Content: `import unittest

import calc


class TestCalc(unittest.TestCase):
    def test_add(self) -> None:
        self.assertEqual(calc.add(2, 2), 4)

    def test_mul(self) -> None:
        self.assertEqual(calc.mul(3, 4), 12)

    def test_div(self) -> None:
        self.assertEqual(calc.div(10, 2), 5)
        with self.assertRaises(ZeroDivisionError):
            calc.div(1, 0)


if __name__ == "__main__":
    unittest.main()
`,
					},
				}},
				Overrides: agenttest.CaseOverrideSpec{AllowedTools: []string{"cli_python"}},
				Prompt:    "Implement div(a: int, b: int) -> int in testsuite/agenttest_fixtures/pysuite/calc.py (raise ZeroDivisionError on division by zero; do not change tests), then run cli_python args [\"test_calc.py\"] with working_directory \"testsuite/agenttest_fixtures/pysuite\".",
				Expect: agenttest.ExpectSpec{
					MustSucceed:          true,
					FilesChanged:         []string{"testsuite/agenttest_fixtures/pysuite/calc.py"},
					ToolCallsMustInclude: []string{"cli_python"},
					ToolCallsMustExclude: []string{"exec_run_tests"},
				},
			},
		}
	case "coding.node":
		suite.Spec.Cases = []agenttest.CaseSpec{
			{
				Name:     "easy_fix_bug_and_run_tests",
				TaskType: string(framework.TaskTypeCodeModification),
				Requires: agenttest.RequiresSpec{
					Executables: []string{"node"},
					Tools:       []string{"cli_node"},
				},
				Setup: agenttest.SetupSpec{Files: []agenttest.SetupFileSpec{
					{
						Path: "testsuite/agenttest_fixtures/nodesuite/sum.js",
						Content: `function add(a, b) {
  return a + b + 1;
}

module.exports = { add };
`,
					},
					{
						Path: "testsuite/agenttest_fixtures/nodesuite/test_sum.js",
						Content: `const assert = require("assert");
const { add } = require("./sum");

assert.strictEqual(add(2, 2), 4);
assert.strictEqual(add(-1, 1), 0);

console.log("ok");
`,
					},
				}},
				Overrides: agenttest.CaseOverrideSpec{AllowedTools: []string{"cli_node"}},
				Prompt:    "Fix the failing tests in testsuite/agenttest_fixtures/nodesuite (do not change test_sum.js), then run cli_node args [\"test_sum.js\"] with working_directory \"testsuite/agenttest_fixtures/nodesuite\".",
				Expect: agenttest.ExpectSpec{
					MustSucceed:          true,
					FilesChanged:         []string{"testsuite/agenttest_fixtures/nodesuite/sum.js"},
					ToolCallsMustInclude: []string{"cli_node"},
					ToolCallsMustExclude: []string{"exec_run_tests"},
				},
			},
			{
				Name:     "medium_add_mul_and_run_tests",
				TaskType: string(framework.TaskTypeCodeModification),
				Requires: agenttest.RequiresSpec{
					Executables: []string{"node"},
					Tools:       []string{"cli_node"},
				},
				Setup: agenttest.SetupSpec{Files: []agenttest.SetupFileSpec{
					{
						Path: "testsuite/agenttest_fixtures/nodesuite/sum.js",
						Content: `function add(a, b) {
  return a + b;
}

module.exports = { add };
`,
					},
					{
						Path: "testsuite/agenttest_fixtures/nodesuite/test_sum.js",
						Content: `const assert = require("assert");
const { add, mul } = require("./sum");

assert.strictEqual(add(2, 2), 4);
assert.strictEqual(mul(3, 4), 12);

console.log("ok");
`,
					},
				}},
				Overrides: agenttest.CaseOverrideSpec{AllowedTools: []string{"cli_node"}},
				Prompt:    "Implement mul(a,b) in testsuite/agenttest_fixtures/nodesuite/sum.js (do not change test_sum.js), then run cli_node args [\"test_sum.js\"] with working_directory \"testsuite/agenttest_fixtures/nodesuite\".",
				Expect: agenttest.ExpectSpec{
					MustSucceed:          true,
					FilesChanged:         []string{"testsuite/agenttest_fixtures/nodesuite/sum.js"},
					ToolCallsMustInclude: []string{"cli_node"},
					ToolCallsMustExclude: []string{"exec_run_tests"},
				},
			},
		}
	case "coding.sqlite":
		suite.Spec.Cases = []agenttest.CaseSpec{
			{
				Name:     "easy_fix_query_and_run",
				TaskType: string(framework.TaskTypeCodeModification),
				Requires: agenttest.RequiresSpec{
					Executables: []string{"sqlite3"},
					Tools:       []string{"cli_sqlite3"},
				},
				Setup: agenttest.SetupSpec{Files: []agenttest.SetupFileSpec{
					{
						Path: "testsuite/agenttest_fixtures/sqlsuite/schema.sql",
						Content: `CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL
);

CREATE TABLE posts (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  title TEXT NOT NULL,
  FOREIGN KEY (user_id) REFERENCES users(id)
);
`,
					},
					{
						Path: "testsuite/agenttest_fixtures/sqlsuite/seed.sql",
						Content: `INSERT INTO users (id, name) VALUES (1, 'alice');
INSERT INTO users (id, name) VALUES (2, 'bob');

INSERT INTO posts (id, user_id, title) VALUES (1, 1, 'hello');
INSERT INTO posts (id, user_id, title) VALUES (2, 1, 'world');
INSERT INTO posts (id, user_id, title) VALUES (3, 2, 'first');
`,
					},
					{
						Path: "testsuite/agenttest_fixtures/sqlsuite/query.sql",
						Content: `-- Return each user name and post count, descending by count then name.
SELECT u.name, COUNT(p.id) AS post_count
FROM users u
LEFT JOIN posts p ON p.user_id = u.id
GROUP BY u.id
ORDER BY u.name ASC;
`,
					},
				}},
				Overrides: agenttest.CaseOverrideSpec{AllowedTools: []string{"cli_sqlite3"}},
				Prompt: `Fix testsuite/agenttest_fixtures/sqlsuite/query.sql so it returns results ordered by post_count DESC then name ASC.

Verify by running cli_sqlite3 args [":memory:"] with working_directory "testsuite/agenttest_fixtures/sqlsuite" and stdin:
.mode list
.separator |
.read schema.sql
.read seed.sql
.read query.sql`,
				Expect: agenttest.ExpectSpec{
					MustSucceed:          true,
					FilesChanged:         []string{"testsuite/agenttest_fixtures/sqlsuite/query.sql"},
					OutputRegex:          []string{`(?m)^alice\\|2$`, `(?m)^bob\\|1$`},
					ToolCallsMustInclude: []string{"cli_sqlite3"},
					ToolCallsMustExclude: []string{"exec_run_tests"},
				},
			},
		}
	}
	data, err := yaml.Marshal(suite)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, data, 0o644)
}

func newAgentTestRunCmd() *cobra.Command {
	var suites []string
	var agentName string
	var outDir string
	var sandbox bool
	var timeout time.Duration
	var model string
	var endpoint string
	var maxIterations int
	var debugLLM bool
	var debugAgent bool
	var ollamaReset string
	var ollamaBin string
	var ollamaService string
	var ollamaResetBetween bool
	var ollamaResetOn []string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one or more agent testsuites",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			if agentName != "" && len(suites) == 0 {
				glob := filepath.Join(ws, "relurpify_cfg", "testsuites", fmt.Sprintf("%s*.testsuite.yaml", sanitizeName(agentName)))
				matches, _ := filepath.Glob(glob)
				suites = matches
			}
			if len(suites) == 0 {
				glob := filepath.Join(ws, "relurpify_cfg", "testsuites", "*.testsuite.yaml")
				matches, _ := filepath.Glob(glob)
				suites = matches
			}
			if len(suites) == 0 {
				return fmt.Errorf("no testsuites found (run `coding-agent agenttest init` first)")
			}
			r := &agenttest.Runner{}
			opts := agenttest.RunOptions{
				TargetWorkspace:    ws,
				OutputDir:          outDir,
				Sandbox:            sandbox,
				Timeout:            timeout,
				ModelOverride:      model,
				EndpointOverride:   endpoint,
				MaxIterations:      maxIterations,
				DebugLLM:           debugLLM,
				DebugAgent:         debugAgent,
				OllamaReset:        ollamaReset,
				OllamaBinary:       ollamaBin,
				OllamaService:      ollamaService,
				OllamaResetBetween: ollamaResetBetween,
				OllamaResetOn:      ollamaResetOn,
			}
			for _, suitePath := range suites {
				suite, err := agenttest.LoadSuite(suitePath)
				if err != nil {
					return err
				}
				ctx := cmd.Context()
				if ctx == nil {
					ctx = context.Background()
				}
				rep, err := r.RunSuite(ctx, suite, opts)
				if err != nil {
					return err
				}
				passed, total, skipped := 0, 0, 0
				for _, c := range rep.Cases {
					if c.Skipped {
						skipped++
						continue
					}
					total++
					if c.Success {
						passed++
					}
				}
				artifactDir := ""
				if len(rep.Cases) > 0 {
					artifactDir = filepath.Dir(rep.Cases[0].ArtifactsDir)
				}
				if skipped > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %d/%d passed (%d skipped) (artifacts: %s)\n", filepath.Base(suitePath), passed, total, skipped, artifactDir)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %d/%d passed (artifacts: %s)\n", filepath.Base(suitePath), passed, total, artifactDir)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&suites, "suite", nil, "Path to a testsuite YAML (repeatable)")
	cmd.Flags().StringVar(&agentName, "agent", "", "Convenience: run relurpify_cfg/testsuites/<agent>.testsuite.yaml")
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory for run artifacts (default: relurpify_cfg/test_runs/...)")
	cmd.Flags().BoolVar(&sandbox, "sandbox", false, "Run tool execution via gVisor/docker (requires runsc + docker)")
	cmd.Flags().DurationVar(&timeout, "timeout", 45*time.Second, "Per-case timeout")
	cmd.Flags().StringVar(&model, "model", "", "Override model name for all cases")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Override Ollama endpoint for all cases")
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 8, "Override max iterations for agent loops")
	cmd.Flags().BoolVar(&debugLLM, "debug-llm", false, "Enable verbose LLM telemetry logging")
	cmd.Flags().BoolVar(&debugAgent, "debug-agent", false, "Enable verbose agent debug logging")
	cmd.Flags().StringVar(&ollamaReset, "ollama-reset", "none", "Reset strategy: none|model|server")
	cmd.Flags().StringVar(&ollamaBin, "ollama-bin", "ollama", "Ollama CLI binary name/path")
	cmd.Flags().StringVar(&ollamaService, "ollama-service", "ollama", "systemd service name for server restarts")
	cmd.Flags().BoolVar(&ollamaResetBetween, "ollama-reset-between", false, "Reset before each case")
	cmd.Flags().StringArrayVar(&ollamaResetOn, "ollama-reset-on", []string{
		"(?i)context deadline exceeded",
		"(?i)connection reset",
		"(?i)EOF",
		"(?i)too many requests",
	}, "Regex patterns that trigger reset+retry (repeatable)")
	return cmd
}

func baseManifestTemplate(wsGlob, model, endpoint string) framework.AgentManifest {
	defaultToolCalling := true
	return framework.AgentManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentManifest",
		Metadata: framework.ManifestMetadata{
			Name:        "coding",
			Version:     "1.0.0",
			Description: "Agent manifest generated by agenttest init",
		},
		Spec: framework.ManifestSpec{
			Image:   "ghcr.io/lexcodex/relurpify/runtime:latest",
			Runtime: "gvisor",
			Permissions: framework.PermissionSet{
				FileSystem: []framework.FileSystemPermission{
					{Action: framework.FileSystemRead, Path: wsGlob, Justification: "Read workspace"},
					{Action: framework.FileSystemList, Path: wsGlob, Justification: "List workspace"},
					{Action: framework.FileSystemWrite, Path: wsGlob, Justification: "Modify workspace"},
					{Action: framework.FileSystemExecute, Path: wsGlob, Justification: "Execute tooling inside workspace"},
				},
				Executables: []framework.ExecutablePermission{
					{Binary: "bash", Args: []string{"-c", "*"}},
					{Binary: "go", Args: []string{"*"}},
					{Binary: "git", Args: []string{"*"}},
				},
				Network: []framework.NetworkPermission{{
					Direction:   "egress",
					Protocol:    "tcp",
					Host:        "localhost",
					Port:        11434,
					Description: "Ollama",
				}},
			},
			Resources: framework.ResourceSpec{
				Limits: framework.ResourceLimit{
					CPU:    "2",
					Memory: "4Gi",
					DiskIO: "200MBps",
				},
			},
			Security: framework.SecuritySpec{
				RunAsUser:       1000,
				ReadOnlyRoot:    false,
				NoNewPrivileges: true,
			},
			Audit: framework.AuditSpec{
				Level:         "verbose",
				RetentionDays: 7,
			},
			Agent: &framework.AgentRuntimeSpec{
				Implementation:    "coding",
				Mode:              framework.AgentModePrimary,
				Version:           "1.0.0",
				Prompt:            defaultAgentPrompt("coding"),
				OllamaToolCalling: &defaultToolCalling,
				Model: framework.AgentModelConfig{
					Provider:    "ollama",
					Name:        model,
					Temperature: 0.2,
					MaxTokens:   4096,
				},
				Tools: framework.AgentToolMatrix{
					FileRead:       true,
					FileWrite:      true,
					FileEdit:       true,
					BashExecute:    true,
					LSPQuery:       false,
					SearchCodebase: true,
				},
			},
		},
	}
}

func setToolMatrixForAgent(m *framework.AgentManifest, name string) {
	if m == nil || m.Spec.Agent == nil {
		return
	}
	switch strings.ToLower(name) {
	case "eternal":
		m.Spec.Agent.Tools = framework.AgentToolMatrix{
			FileRead:       false,
			FileWrite:      false,
			FileEdit:       false,
			BashExecute:    false,
			LSPQuery:       false,
			SearchCodebase: false,
		}
	default:
	}
}

func agentImplementation(name string) string {
	switch strings.ToLower(name) {
	case "planner":
		return "planner"
	case "react":
		return "react"
	case "reflection":
		return "reflection"
	case "expert":
		return "expert"
	case "eternal":
		return "eternal"
	default:
		return "coding"
	}
}

func defaultAgentPrompt(name string) string {
	return fmt.Sprintf("You are %s. Follow project rules, ask before destructive actions, and summarize outcomes.", strings.ToLower(name))
}

func defaultSuite(workspace, agentName, endpoint, model string) agenttest.Suite {
	// Suites live under relurpify_cfg/testsuites/, so keep manifest path relative.
	manifest := filepath.ToSlash(filepath.Join("..", "agents", fmt.Sprintf("%s.yaml", sanitizeName(agentName))))
	suite := agenttest.Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata: agenttest.SuiteMeta{
			Name:        agentName,
			Description: fmt.Sprintf("Default tests for %s", agentName),
		},
		Spec: agenttest.SuiteSpec{
			AgentName: agentName,
			Manifest:  manifest,
			Workspace: agenttest.WorkspaceSpec{
				Strategy: "in_place",
				Exclude: []string{
					".git/**",
					".gocache/**",
					".gomodcache/**",
					"relurpify_cfg/test_runs/**",
				},
			},
			Models: []agenttest.ModelSpec{{Name: model, Endpoint: endpoint}},
			Recording: agenttest.RecordingSpec{
				Mode: "off",
			},
		},
	}
	switch strings.ToLower(agentName) {
	case "eternal":
		suite.Spec.Cases = []agenttest.CaseSpec{{
			Name:     "one_cycle_ascii",
			TaskType: string(framework.TaskTypeCodeGeneration),
			Prompt:   "write a 3 line ascii cat then stop",
			Context: map[string]any{
				"eternal.infinite":   false,
				"eternal.max_cycles": 1,
				"eternal.sleep":      "0s",
			},
			Expect: agenttest.ExpectSpec{
				MustSucceed:  true,
				OutputRegex:  []string{`(?i)(cat|/\\_/\\|meow)`},
				MaxToolCalls: 0,
			},
		}}
	default:
		suite.Spec.Cases = []agenttest.CaseSpec{
			{
				Name:     "summarize_readme",
				TaskType: string(framework.TaskTypeCodeGeneration),
				Prompt:   "Summarize README.md in 5 bullets.",
				Context:  map[string]any{"path": "README.md"},
				Expect: agenttest.ExpectSpec{
					MustSucceed:          true,
					OutputRegex:          []string{`(?i)relurpify`},
					ToolCallsMustExclude: []string{"file_write", "file_edit", "file_create", "file_delete"},
				},
			},
			{
				Name:     "edit_fixture_file",
				TaskType: string(framework.TaskTypeCodeModification),
				Setup: agenttest.SetupSpec{Files: []agenttest.SetupFileSpec{{
					Path:    "testsuite/agenttest_fixtures/hello.txt",
					Content: "hello\n",
				}}},
				Prompt: "Edit testsuite/agenttest_fixtures/hello.txt by appending a new line containing 'DONE'.",
				Expect: agenttest.ExpectSpec{
					MustSucceed: true,
					FilesChanged: []string{
						"testsuite/agenttest_fixtures/hello.txt",
					},
					ToolCallsMustInclude: []string{"file_write"},
				},
			},
		}
	}
	return suite
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
