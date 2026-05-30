package subprocess

import (
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

func TestExpandCommandBaseOnly(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"rg"},
			},
		},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"rg"}, cmd)
}

func TestExpandCommandBaseWithArgs(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"echo"},
				Args: []string{"hello", "world"},
			},
		},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"echo", "hello", "world"}, cmd)
}

func TestExpandCommandWithPlaceholders(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"echo"},
				Args: []string{"${message}"},
			},
		},
	}, map[string]interface{}{
		"message": "hello",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"echo", "hello"}, cmd)
}

func TestExpandCommandWithDefaultArgs(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend:     contracts.ToolBackendSubprocess,
			Command:     &contracts.ToolManifestCommand{Base: []string{"echo"}},
			DefaultArgs: []string{"-n"},
		},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"echo", "-n"}, cmd)
}

func TestExpandCommandWithRawArgs(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"echo"}},
			Sandbox: &contracts.ToolManifestSandbox{AllowFlags: true},
		},
	}, map[string]interface{}{
		"args": []interface{}{"hello", "world"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"echo", "hello", "world"}, cmd)
}

func TestExpandCommandRawArgsFlagGuard(t *testing.T) {
	_, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"echo"}},
			// allow_flags defaults to false
		},
	}, map[string]interface{}{
		"args": []interface{}{"--version"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "flag injection")
}

func TestExpandCommandRawArgsFlagAllowed(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"echo"}},
			Sandbox: &contracts.ToolManifestSandbox{AllowFlags: true},
		},
	}, map[string]interface{}{
		"args": []interface{}{"--verbose"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"echo", "--verbose"}, cmd)
}

func TestExpandCommandMissingPlaceholder(t *testing.T) {
	_, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"echo"},
				Args: []string{"${missing}"},
			},
		},
	}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), `missing parameter "missing"`)
}

// --- Typed flag tests ---

func TestTypedFlagEqualsStyle(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]contracts.ToolManifestFlag{
					"output": {
						Param: "output_path",
						Style: contracts.FlagStyleEquals,
					},
				},
			},
		},
	}, map[string]interface{}{
		"output_path": "result.json",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"rg", "--output=result.json"}, cmd)
}

func TestTypedFlagEqualsStyleDefaults(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]contracts.ToolManifestFlag{
					"output": {
						Param: "output_path",
						// Style empty — defaults to equals
					},
				},
			},
		},
	}, map[string]interface{}{
		"output_path": "result.json",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"rg", "--output=result.json"}, cmd)
}

func TestTypedFlagSeparateStyle(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]contracts.ToolManifestFlag{
					"output": {
						Param: "output_path",
						Style: contracts.FlagStyleSeparate,
					},
				},
			},
		},
	}, map[string]interface{}{
		"output_path": "result.json",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"rg", "--output", "result.json"}, cmd)
}

func TestTypedFlagRepeatEquals(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]contracts.ToolManifestFlag{
					"glob": {
						Param:  "patterns",
						Style:  contracts.FlagStyleEquals,
						Repeat: true,
					},
				},
			},
		},
	}, map[string]interface{}{
		"patterns": []interface{}{"*.go", "*.rs"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"rg", "--glob=*.go", "--glob=*.rs"}, cmd)
}

func TestTypedFlagRepeatSeparate(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]contracts.ToolManifestFlag{
					"glob": {
						Param:  "patterns",
						Style:  contracts.FlagStyleSeparate,
						Repeat: true,
					},
				},
			},
		},
	}, map[string]interface{}{
		"patterns": []interface{}{"*.go", "*.rs"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"rg", "--glob", "*.go", "--glob", "*.rs"}, cmd)
}

func TestTypedFlagSeparateStyleWithAdversarialValue(t *testing.T) {
	// Value containing =, space-like chars must stay a single token
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]contracts.ToolManifestFlag{
					"pattern": {
						Param: "pat",
						Style: contracts.FlagStyleSeparate,
					},
				},
			},
		},
	}, map[string]interface{}{
		"pat": "a=b;c d",
	})
	require.NoError(t, err)
	require.Len(t, cmd, 3, "must produce exactly 3 tokens")
	require.Equal(t, "rg", cmd[0])
	require.Equal(t, "--pattern", cmd[1])
	require.Equal(t, "a=b;c d", cmd[2])
}

// --- Boolean flag tests ---

func TestBooleanFlagTrue(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"colordiff"},
				Flags: map[string]contracts.ToolManifestFlag{
					"color": {
						WhenTrue:  []string{"--color=always"},
						WhenFalse: []string{"--color=never"},
					},
				},
			},
		},
	}, map[string]interface{}{
		"color": true,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"colordiff", "--color=always"}, cmd)
}

func TestBooleanFlagFalse(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"colordiff"},
				Flags: map[string]contracts.ToolManifestFlag{
					"color": {
						WhenTrue:  []string{"--color=always"},
						WhenFalse: []string{"--color=never"},
					},
				},
			},
		},
	}, map[string]interface{}{
		"color": false,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"colordiff", "--color=never"}, cmd)
}

func TestBooleanFlagNotProvided(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"colordiff"},
				Flags: map[string]contracts.ToolManifestFlag{
					"color": {
						WhenTrue:  []string{"--color=always"},
						WhenFalse: []string{"--color=never"},
					},
				},
			},
		},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"colordiff"}, cmd)
}

func TestBooleanFlagOnlyWhenTrue(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"tool"},
				Flags: map[string]contracts.ToolManifestFlag{
					"verbose": {
						WhenTrue: []string{"--verbose"},
					},
				},
			},
		},
	}, map[string]interface{}{
		"verbose": true,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"tool", "--verbose"}, cmd)
}

func TestBooleanFlagOnlyWhenTrueFalse(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"tool"},
				Flags: map[string]contracts.ToolManifestFlag{
					"verbose": {
						WhenTrue: []string{"--verbose"},
					},
				},
			},
		},
	}, map[string]interface{}{
		"verbose": false,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"tool"}, cmd)
}

// --- Combined tests ---

func TestTypedAndBooleanFlagsCombined(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]contracts.ToolManifestFlag{
					"output": {
						Param: "output_path",
						Style: contracts.FlagStyleEquals,
					},
					"hidden": {
						WhenTrue: []string{"--hidden"},
					},
				},
			},
		},
	}, map[string]interface{}{
		"output_path": "out.json",
		"hidden":      true,
	})
	require.NoError(t, err)
	// Flags sorted: hidden < output
	require.Equal(t, []string{"rg", "--hidden", "--output=out.json"}, cmd)
}

func TestExpandCommandAllComponents(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Parameters: []contracts.ToolParameter{
			{Name: "pattern", Type: "string", Required: true},
		},
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"rg"},
				Args: []string{"${pattern}"},
				Flags: map[string]contracts.ToolManifestFlag{
					"glob": {
						Param:  "globs",
						Style:  contracts.FlagStyleSeparate,
						Repeat: true,
					},
				},
			},
			DefaultArgs: []string{"--no-ignore"},
			Sandbox:     &contracts.ToolManifestSandbox{AllowFlags: true},
		},
	}, map[string]interface{}{
		"pattern": "func",
		"globs":   []interface{}{"*.go", "*.rs"},
		"args":    []interface{}{"--follow"},
	})
	require.NoError(t, err)
	// Order: base + flags(glob) + positional(pattern) + default_args + raw
	require.Equal(t, []string{"rg", "--glob", "*.go", "--glob", "*.rs", "func", "--no-ignore", "--follow"}, cmd)
}

// --- Error cases ---

func TestMissingRequiredParamViaPlaceholder(t *testing.T) {
	_, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"echo"},
				Args: []string{"${required_param}"},
			},
		},
	}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), `missing parameter "required_param"`)
}

func TestPartialPlaceholderTokenRejected(t *testing.T) {
	_, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"echo"},
				Args: []string{"prefix_${x}_suffix"},
			},
		},
	}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be a single placeholder token")
}

func TestNoCommandBase(t *testing.T) {
	_, err := ExpandCommand(contracts.ToolManifest{
		Name: "empty",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{},
		},
	}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "execution.command.base required")
}

func TestNoCommandSpec(t *testing.T) {
	_, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
		},
	}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "execution.command required")
}

// --- Fuzz-like adversarial value tests ---

func TestAdversarialValuesStaySingleTokens(t *testing.T) {
	adversarial := []string{
		"a=b",
		"a;b",
		"a b",
		"a\tb",
		"a\nb",
		"$(id)",
		"`id`",
		"'; rm -rf /'",
		"\"& hover=1\"",
		"value with spaces",
		"--flag-value",
	}

	for _, val := range adversarial {
		t.Run(val, func(t *testing.T) {
			cmd, err := ExpandCommand(contracts.ToolManifest{
				Execution: contracts.ToolManifestExecution{
					Backend: contracts.ToolBackendSubprocess,
					Command: &contracts.ToolManifestCommand{
						Base: []string{"tool"},
						Args: []string{"${x}"},
					},
				},
			}, map[string]interface{}{
				"x": val,
			})
			require.NoError(t, err)
			require.Len(t, cmd, 2, "must produce exactly 2 tokens: base + one value")
			require.Equal(t, val, cmd[1], "adversarial value must be preserved as-is")
		})
	}
}

func TestParamNotProvidedTypedFlagSkipped(t *testing.T) {
	cmd, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]contracts.ToolManifestFlag{
					"output": {
						Param: "output_path",
						Style: contracts.FlagStyleEquals,
					},
				},
			},
		},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"rg"}, cmd)
}

func TestBoolFlagNonBoolValueError(t *testing.T) {
	_, err := ExpandCommand(contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"tool"},
				Flags: map[string]contracts.ToolManifestFlag{
					"verbose": {
						WhenTrue: []string{"--verbose"},
					},
				},
			},
		},
	}, map[string]interface{}{
		"verbose": "not-a-bool",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "verbose" expects a boolean value`)
}
