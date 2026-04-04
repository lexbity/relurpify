package shell

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lexcodex/relurpify/framework/sandbox"
	"gopkg.in/yaml.v3"
)

// ShellBinding is a user-defined named shell capability loaded from
// relurpify_cfg/skills/shell_bindings.yaml.
type ShellBinding struct {
	ID              string   `yaml:"id"`
	Name            string   `yaml:"name"`
	Description     string   `yaml:"description"`
	Command         []string `yaml:"command"`
	Shell           bool     `yaml:"shell"`
	ArgsPassthrough bool     `yaml:"args_passthrough"`
	Tags            []string `yaml:"tags"`
}

// shellBindingsYAML is the raw YAML structure.
type shellBindingsYAML struct {
	Version  string         `yaml:"version"`
	Bindings []ShellBinding `yaml:"bindings"`
}

// CommandQuery resolves named bindings or validates raw args against the allowed
// binary set, then produces a sandbox.CommandRequest.
// Shell bindings with Shell:true are rejected when shell is not in the allowed set.
type CommandQuery struct {
	allowed  map[string]struct{} // binaries permitted per capability registry
	shellOK  bool                // true if sh or bash is in the allowed set
	bindings map[string]*ShellBinding
}

// NewCommandQuery creates a new CommandQuery.
func NewCommandQuery(allowed []string, bindings []ShellBinding) *CommandQuery {
	allowedSet := make(map[string]struct{})
	shellOK := false
	for _, bin := range allowed {
		allowedSet[bin] = struct{}{}
		if bin == "sh" || bin == "bash" {
			shellOK = true
		}
	}
	// If allowed list is empty, assume all binaries are allowed (including shell)
	if len(allowed) == 0 {
		shellOK = true
	}
	bindingMap := make(map[string]*ShellBinding)
	for i := range bindings {
		bindingMap[bindings[i].Name] = &bindings[i]
	}
	return &CommandQuery{
		allowed:  allowedSet,
		shellOK:  shellOK,
		bindings: bindingMap,
	}
}

// Resolve returns a CommandRequest for a named binding with optional extra args.
func (q *CommandQuery) Resolve(name string, extraArgs []string) (sandbox.CommandRequest, error) {
	binding, exists := q.bindings[name]
	if !exists {
		return sandbox.CommandRequest{}, fmt.Errorf("shell binding %q not found", name)
	}
	// Check shell permission
	if binding.Shell && !q.shellOK {
		// If allowed set is empty, we assume shell is allowed (backwards compatibility)
		if len(q.allowed) > 0 {
			return sandbox.CommandRequest{}, fmt.Errorf("shell binding %q requires shell execution but shell is not allowed", name)
		}
	}
	// Build args
	args := make([]string, len(binding.Command))
	copy(args, binding.Command)
	if binding.ArgsPassthrough {
		args = append(args, extraArgs...)
	}
	// If Shell is true, wrap in bash -c
	if binding.Shell {
		if len(args) != 1 {
			return sandbox.CommandRequest{}, fmt.Errorf("shell binding %q with shell:true must have exactly one command element", name)
		}
		args = []string{"bash", "-c", args[0]}
	}
	// Validate each binary against allowed set (skip if allowed set is empty)
	if len(q.allowed) > 0 {
		for i, arg := range args {
			if i == 0 {
				bin := filepath.Base(arg)
				if _, allowed := q.allowed[bin]; !allowed {
					return sandbox.CommandRequest{}, fmt.Errorf("binary %q in binding %q is not allowed", bin, name)
				}
			}
		}
	}
	return sandbox.CommandRequest{
		Args: args,
	}, nil
}

// ValidateRaw returns a CommandRequest for a raw argv slice, checking each
// binary against the allowed set. Never produces a bash -c command.
func (q *CommandQuery) ValidateRaw(args []string) (sandbox.CommandRequest, error) {
	if len(args) == 0 {
		return sandbox.CommandRequest{}, fmt.Errorf("empty command")
	}
	// Skip validation if allowed set is empty (backwards compatibility)
	if len(q.allowed) > 0 {
		bin := filepath.Base(args[0])
		if _, allowed := q.allowed[bin]; !allowed {
			return sandbox.CommandRequest{}, fmt.Errorf("binary %q is not allowed", bin)
		}
	}
	return sandbox.CommandRequest{
		Args: args,
	}, nil
}

// LoadShellBindings reads bindings from a YAML file.
func LoadShellBindings(path string) ([]ShellBinding, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open bindings file: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read bindings file: %w", err)
	}
	var raw shellBindingsYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse bindings YAML: %w", err)
	}
	return raw.Bindings, nil
}
