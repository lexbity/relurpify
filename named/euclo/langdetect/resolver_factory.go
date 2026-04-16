package langdetect

import (
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	golangpkg "github.com/lexcodex/relurpify/platform/lang/go"
	jspkg "github.com/lexcodex/relurpify/platform/lang/js"
	pythonpkg "github.com/lexcodex/relurpify/platform/lang/python"
	rustpkg "github.com/lexcodex/relurpify/platform/lang/rust"
)

// ResolverFactory builds framework planners from detected workspace languages.
// It is the single place in named/euclo that imports platform/lang/* packages.
type ResolverFactory struct {
	Languages WorkspaceLanguages
}

var (
	newGoVerificationResolver = func() frameworkplan.VerificationBackendResolver {
		return golangpkg.NewVerificationResolver()
	}
	newPythonVerificationResolver = func() frameworkplan.VerificationBackendResolver {
		return pythonpkg.NewVerificationResolver()
	}
	newRustVerificationResolver = func() frameworkplan.VerificationBackendResolver {
		return rustpkg.NewVerificationResolver()
	}
	newJSVerificationResolver = func() frameworkplan.VerificationBackendResolver {
		return jspkg.NewVerificationResolver()
	}
	newGoCompatibilityResolver = func() frameworkplan.CompatibilityBackendResolver {
		return golangpkg.NewCompatibilitySurfaceResolver()
	}
	newPythonCompatibilityResolver = func() frameworkplan.CompatibilityBackendResolver {
		return pythonpkg.NewCompatibilitySurfaceResolver()
	}
	newRustCompatibilityResolver = func() frameworkplan.CompatibilityBackendResolver {
		return rustpkg.NewCompatibilitySurfaceResolver()
	}
	newJSCompatibilityResolver = func() frameworkplan.CompatibilityBackendResolver {
		return jspkg.NewCompatibilitySurfaceResolver()
	}
)

// VerificationPlanner builds a VerificationScopePlanner for the detected languages.
// If no language is detected, it falls back to all languages.
func (f ResolverFactory) VerificationPlanner() *frameworkplan.VerificationScopePlanner {
	langs := f.Languages
	if langs.IsEmpty() {
		langs = WorkspaceLanguages{Go: true, Python: true, Rust: true, JS: true}
	}
	resolvers := make([]frameworkplan.VerificationBackendResolver, 0, 4)
	if langs.Go {
		resolvers = append(resolvers, newGoVerificationResolver())
	}
	if langs.Python {
		resolvers = append(resolvers, newPythonVerificationResolver())
	}
	if langs.Rust {
		resolvers = append(resolvers, newRustVerificationResolver())
	}
	if langs.JS {
		resolvers = append(resolvers, newJSVerificationResolver())
	}
	return frameworkplan.NewVerificationScopePlanner(resolvers...)
}

// CompatibilitySurfacePlanner builds a CompatibilitySurfacePlanner for the
// detected languages. If no language is detected, it falls back to all languages.
func (f ResolverFactory) CompatibilitySurfacePlanner() *frameworkplan.CompatibilitySurfacePlanner {
	langs := f.Languages
	if langs.IsEmpty() {
		langs = WorkspaceLanguages{Go: true, Python: true, Rust: true, JS: true}
	}
	resolvers := make([]frameworkplan.CompatibilityBackendResolver, 0, 4)
	if langs.Go {
		resolvers = append(resolvers, newGoCompatibilityResolver())
	}
	if langs.Python {
		resolvers = append(resolvers, newPythonCompatibilityResolver())
	}
	if langs.Rust {
		resolvers = append(resolvers, newRustCompatibilityResolver())
	}
	if langs.JS {
		resolvers = append(resolvers, newJSCompatibilityResolver())
	}
	return frameworkplan.NewCompatibilitySurfacePlanner(resolvers...)
}
