package subprocess

import (
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

const (
	color        = "color"
	color_always = "--color=always"
	color_never  = "--color=never"
	colordiff    = "colordiff"
	echo         = "echo"
	glob         = "--glob"
	glob_2       = "glob"
	goVar        = "*.go"
	hello        = "hello"
	output       = "output"
	output_path  = "output_path"
	pattern      = "pattern"
	patterns     = "patterns"
	result_json  = "result.json"
	rg           = "rg"
	rs           = "*.rs"
	verbose      = "verbose"
	verbose_2    = "--verbose"
	world        = "world"
)

func TestExpandCommandBaseOnly(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{rg},
			},
		},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{rg}, cmd)
}

func TestExpandCommandBaseWithArgs(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{echo},
				Args: []string{hello, world},
			},
		},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{echo, hello, world}, cmd)
}

func TestExpandCommandWithPlaceholders(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{echo},
				Args: []string{"${message}"},
			},
		},
	}, map[string]any{
		"message": hello,
	})
	require.NoError(t, err)
	require.Equal(t, []string{echo, hello}, cmd)
}

func TestExpandCommandWithDefaultArgs(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend:     ports.ToolBackendSubprocess,
			Command:     &ports.ToolManifestCommand{Base: []string{echo}},
			DefaultArgs: []string{"-n"},
		},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{echo, "-n"}, cmd)
}

func TestExpandCommandWithRawArgs(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{echo}},
			Sandbox: &ports.ToolManifestSandbox{AllowFlags: true},
		},
	}, map[string]any{
		args: []any{hello, world},
	})
	require.NoError(t, err)
	require.Equal(t, []string{echo, hello, world}, cmd)
}

func TestExpandCommandRawArgsFlagGuard(t *testing.T) {
	_, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{echo}},
			// allow_flags defaults to false
		},
	}, map[string]any{
		args: []any{"--version"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "flag injection")
}

func TestExpandCommandRawArgsFlagAllowed(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{echo}},
			Sandbox: &ports.ToolManifestSandbox{AllowFlags: true},
		},
	}, map[string]any{
		args: []any{verbose_2},
	})
	require.NoError(t, err)
	require.Equal(t, []string{echo, verbose_2}, cmd)
}

func TestExpandCommandMissingPlaceholder(t *testing.T) {
	_, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{echo},
				Args: []string{"${missing}"},
			},
		},
	}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), `missing parameter "missing"`)
}

// --- Typed flag tests ---

func TestTypedFlagEqualsStyle(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{rg},
				Flags: map[string]ports.ToolManifestFlag{
					output: {
						Param: output_path,
						Style: ports.FlagStyleEquals,
					},
				},
			},
		},
	}, map[string]any{
		output_path: result_json,
	})
	require.NoError(t, err)
	require.Equal(t, []string{rg, "--output=result.json"}, cmd)
}

func TestTypedFlagEqualsStyleDefaults(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{rg},
				Flags: map[string]ports.ToolManifestFlag{
					output: {
						Param: output_path,
						// Style empty — defaults to equals
					},
				},
			},
		},
	}, map[string]any{
		output_path: result_json,
	})
	require.NoError(t, err)
	require.Equal(t, []string{rg, "--output=result.json"}, cmd)
}

func TestTypedFlagSeparateStyle(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{rg},
				Flags: map[string]ports.ToolManifestFlag{
					output: {
						Param: output_path,
						Style: ports.FlagStyleSeparate,
					},
				},
			},
		},
	}, map[string]any{
		output_path: result_json,
	})
	require.NoError(t, err)
	require.Equal(t, []string{rg, "--output", result_json}, cmd)
}

func TestTypedFlagRepeatEquals(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{rg},
				Flags: map[string]ports.ToolManifestFlag{
					glob_2: {
						Param:  patterns,
						Style:  ports.FlagStyleEquals,
						Repeat: true,
					},
				},
			},
		},
	}, map[string]any{
		patterns: []any{goVar, rs},
	})
	require.NoError(t, err)
	require.Equal(t, []string{rg, "--glob=*.go", "--glob=*.rs"}, cmd)
}

func TestTypedFlagRepeatSeparate(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{rg},
				Flags: map[string]ports.ToolManifestFlag{
					glob_2: {
						Param:  patterns,
						Style:  ports.FlagStyleSeparate,
						Repeat: true,
					},
				},
			},
		},
	}, map[string]any{
		patterns: []any{goVar, rs},
	})
	require.NoError(t, err)
	require.Equal(t, []string{rg, glob, goVar, glob, rs}, cmd)
}

func TestTypedFlagSeparateStyleWithAdversarialValue(t *testing.T) {
	// Value containing =, space-like chars must stay a single token
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{rg},
				Flags: map[string]ports.ToolManifestFlag{
					pattern: {
						Param: "pat",
						Style: ports.FlagStyleSeparate,
					},
				},
			},
		},
	}, map[string]any{
		"pat": "a=b;c d",
	})
	require.NoError(t, err)
	require.Len(t, cmd, 3, "must produce exactly 3 tokens")
	require.Equal(t, rg, cmd[0])
	require.Equal(t, "--pattern", cmd[1])
	require.Equal(t, "a=b;c d", cmd[2])
}

// --- Boolean flag tests ---

func TestBooleanFlagTrue(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{colordiff},
				Flags: map[string]ports.ToolManifestFlag{
					color: {
						WhenTrue:  []string{color_always},
						WhenFalse: []string{color_never},
					},
				},
			},
		},
	}, map[string]any{
		color: true,
	})
	require.NoError(t, err)
	require.Equal(t, []string{colordiff, color_always}, cmd)
}

func TestBooleanFlagFalse(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{colordiff},
				Flags: map[string]ports.ToolManifestFlag{
					color: {
						WhenTrue:  []string{color_always},
						WhenFalse: []string{color_never},
					},
				},
			},
		},
	}, map[string]any{
		color: false,
	})
	require.NoError(t, err)
	require.Equal(t, []string{colordiff, color_never}, cmd)
}

func TestBooleanFlagNotProvided(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{colordiff},
				Flags: map[string]ports.ToolManifestFlag{
					color: {
						WhenTrue:  []string{color_always},
						WhenFalse: []string{color_never},
					},
				},
			},
		},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{colordiff}, cmd)
}

func TestBooleanFlagOnlyWhenTrue(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{tool},
				Flags: map[string]ports.ToolManifestFlag{
					verbose: {
						WhenTrue: []string{verbose_2},
					},
				},
			},
		},
	}, map[string]any{
		verbose: true,
	})
	require.NoError(t, err)
	require.Equal(t, []string{tool, verbose_2}, cmd)
}

func TestBooleanFlagOnlyWhenTrueFalse(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{tool},
				Flags: map[string]ports.ToolManifestFlag{
					verbose: {
						WhenTrue: []string{verbose_2},
					},
				},
			},
		},
	}, map[string]any{
		verbose: false,
	})
	require.NoError(t, err)
	require.Equal(t, []string{tool}, cmd)
}

// --- Combined tests ---

func TestTypedAndBooleanFlagsCombined(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{rg},
				Flags: map[string]ports.ToolManifestFlag{
					output: {
						Param: output_path,
						Style: ports.FlagStyleEquals,
					},
					"hidden": {
						WhenTrue: []string{"--hidden"},
					},
				},
			},
		},
	}, map[string]any{
		output_path: "out.json",
		"hidden":    true,
	})
	require.NoError(t, err)
	// Flags sorted: hidden < output
	require.Equal(t, []string{rg, "--hidden", "--output=out.json"}, cmd)
}

func TestExpandCommandAllComponents(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Parameters: []ports.ToolParameter{
			{Name: pattern, Type: "string", Required: true},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{rg},
				Args: []string{"${pattern}"},
				Flags: map[string]ports.ToolManifestFlag{
					glob_2: {
						Param:  "globs",
						Style:  ports.FlagStyleSeparate,
						Repeat: true,
					},
				},
			},
			DefaultArgs: []string{"--no-ignore"},
			Sandbox:     &ports.ToolManifestSandbox{AllowFlags: true},
		},
	}, map[string]any{
		pattern: "func",
		"globs": []any{goVar, rs},
		args:    []any{"--follow"},
	})
	require.NoError(t, err)
	// Order: base + flags(glob) + positional(pattern) + default_args + raw
	require.Equal(t, []string{rg, glob, goVar, glob, rs, "func", "--no-ignore", "--follow"}, cmd)
}

// --- Error cases ---

func TestMissingRequiredParamViaPlaceholder(t *testing.T) {
	_, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{echo},
				Args: []string{"${required_param}"},
			},
		},
	}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), `missing parameter "required_param"`)
}

func TestPartialPlaceholderTokenRejected(t *testing.T) {
	_, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{echo},
				Args: []string{"prefix_${x}_suffix"},
			},
		},
	}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be a single placeholder token")
}

func TestNoCommandBase(t *testing.T) {
	_, err := ExpandCommand(ports.ToolManifest{
		Name: "empty",
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{},
		},
	}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "execution.command.base required")
}

func TestNoCommandSpec(t *testing.T) {
	_, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
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
			cmd, err := ExpandCommand(ports.ToolManifest{
				Execution: ports.ToolManifestExecution{
					Backend: ports.ToolBackendSubprocess,
					Command: &ports.ToolManifestCommand{
						Base: []string{tool},
						Args: []string{"${x}"},
					},
				},
			}, map[string]any{
				"x": val,
			})
			require.NoError(t, err)
			require.Len(t, cmd, 2, "must produce exactly 2 tokens: base + one value")
			require.Equal(t, val, cmd[1], "adversarial value must be preserved as-is")
		})
	}
}

func TestParamNotProvidedTypedFlagSkipped(t *testing.T) {
	cmd, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{rg},
				Flags: map[string]ports.ToolManifestFlag{
					output: {
						Param: output_path,
						Style: ports.FlagStyleEquals,
					},
				},
			},
		},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{rg}, cmd)
}

func TestBoolFlagNonBoolValueError(t *testing.T) {
	_, err := ExpandCommand(ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{tool},
				Flags: map[string]ports.ToolManifestFlag{
					verbose: {
						WhenTrue: []string{verbose_2},
					},
				},
			},
		},
	}, map[string]any{
		verbose: "not-a-bool",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag verbose expects a boolean value`)
}
