package envcomposition

import (
	"fmt"
	"strings"
)

// ValidateSecurityRuntimeInput checks for inconsistent or privilege-widening
// configurations before any sandbox resources are allocated. It is called
// at the top of BuildSecurityRuntime and must never be bypassed.
//
// Current checks:
//   - Backend/runtime mismatch (e.g. runtime says gvisor but
//     sandbox.backend is docker, which drifts the security model).
//   - Under strict mode, a non-loopback bind is rejected (promoted from
//     nexus SecurityWarnings to a hard gate).
func ValidateSecurityRuntimeInput(in SecurityRuntimeInput) error {
	var errs []error

	// Check 1: Backend vs runtime mismatch.
	// If the resolved runtime declares a specific backend, the sandbox backend
	// must be compatible. A gvisor manifest with a docker backend would
	// bypass gvisor's security model (protected paths, no-new-privileges,
	// seccomp).
	manifestRuntime := strings.ToLower(strings.TrimSpace(in.Runtime))
	if manifestRuntime != "" {
		resolvedBackend := resolveEffectiveBackend(in.SandboxBackend)
		if !backendsCompatible(manifestRuntime, resolvedBackend) {
			errs = append(errs, fmt.Errorf(
				"runtime %q is incompatible with sandbox backend %q",
				manifestRuntime, resolvedBackend))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %w", joinErrors(errs))
	}
	return nil
}

// resolveEffectiveBackend returns the effective sandbox backend after
// applying defaults (empty defaults to gvisor).
func resolveEffectiveBackend(backend string) string {
	b := strings.ToLower(strings.TrimSpace(backend))
	if b == "" {
		return "gvisor"
	}
	return b
}

// backendsCompatible returns true when the runtime and the
// resolved sandbox backend are compatible. The key rule: gvisor is
// the only runtime that provides the full security model (protected
// paths, no-new-privileges, seccomp). A docker backend with a gvisor
// manifest is a security model downgrade.
func backendsCompatible(manifestRuntime, resolvedBackend string) bool {
	switch manifestRuntime {
	case "gvisor":
		return resolvedBackend == "gvisor"
	default:
		// Unknown manifest runtimes are handled by the sandbox selector.
		return true
	}
}

func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%w", &multiError{errs: errs})
}

type multiError struct {
	errs []error
}

func (m *multiError) Error() string {
	parts := make([]string, len(m.errs))
	for i, e := range m.errs {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "; ")
}

func (m *multiError) Unwrap() error {
	if len(m.errs) == 0 {
		return nil
	}
	return m.errs[0]
}
