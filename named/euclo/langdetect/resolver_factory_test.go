package langdetect

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

type stubVerificationResolver struct {
	backendID string
}

func (s stubVerificationResolver) BackendID() string { return s.backendID }

func (s stubVerificationResolver) Supports(agentenv.VerificationPlanRequest) bool { return true }

func (s stubVerificationResolver) BuildPlan(context.Context, agentenv.VerificationPlanRequest) (agentenv.VerificationPlan, bool, error) {
	return agentenv.VerificationPlan{Commands: []agentenv.VerificationCommand{{Command: "stub"}}}, true, nil
}

type stubCompatibilityResolver struct {
	backendID string
}

func (s stubCompatibilityResolver) BackendID() string { return s.backendID }

func (s stubCompatibilityResolver) Supports(agentenv.CompatibilitySurfaceRequest) bool { return true }

func (s stubCompatibilityResolver) ExtractSurface(context.Context, agentenv.CompatibilitySurfaceRequest) (agentenv.CompatibilitySurface, bool, error) {
	return agentenv.CompatibilitySurface{}, true, nil
}

func TestResolverFactoryVerificationPlanner_GoOnly(t *testing.T) {
	originalGo := newGoVerificationResolver
	originalPython := newPythonVerificationResolver
	originalRust := newRustVerificationResolver
	originalJS := newJSVerificationResolver
	t.Cleanup(func() {
		newGoVerificationResolver = originalGo
		newPythonVerificationResolver = originalPython
		newRustVerificationResolver = originalRust
		newJSVerificationResolver = originalJS
	})

	var goCount, pythonCount, rustCount, jsCount int
	newGoVerificationResolver = func() frameworkplan.VerificationBackendResolver {
		goCount++
		return stubVerificationResolver{backendID: "go"}
	}
	newPythonVerificationResolver = func() frameworkplan.VerificationBackendResolver {
		pythonCount++
		return stubVerificationResolver{backendID: "python"}
	}
	newRustVerificationResolver = func() frameworkplan.VerificationBackendResolver {
		rustCount++
		return stubVerificationResolver{backendID: "rust"}
	}
	newJSVerificationResolver = func() frameworkplan.VerificationBackendResolver {
		jsCount++
		return stubVerificationResolver{backendID: "javascript"}
	}

	planner := ResolverFactory{Languages: WorkspaceLanguages{Go: true}}.VerificationPlanner()
	if planner == nil {
		t.Fatal("expected planner to be non-nil")
	}
	if goCount != 1 || pythonCount != 0 || rustCount != 0 || jsCount != 0 {
		t.Fatalf("unexpected constructor counts: go=%d python=%d rust=%d js=%d", goCount, pythonCount, rustCount, jsCount)
	}
}

func TestResolverFactoryCompatibilityPlanner_GoOnly(t *testing.T) {
	originalGo := newGoCompatibilityResolver
	originalPython := newPythonCompatibilityResolver
	originalRust := newRustCompatibilityResolver
	originalJS := newJSCompatibilityResolver
	t.Cleanup(func() {
		newGoCompatibilityResolver = originalGo
		newPythonCompatibilityResolver = originalPython
		newRustCompatibilityResolver = originalRust
		newJSCompatibilityResolver = originalJS
	})

	var goCount, pythonCount, rustCount, jsCount int
	newGoCompatibilityResolver = func() frameworkplan.CompatibilityBackendResolver {
		goCount++
		return stubCompatibilityResolver{backendID: "go"}
	}
	newPythonCompatibilityResolver = func() frameworkplan.CompatibilityBackendResolver {
		pythonCount++
		return stubCompatibilityResolver{backendID: "python"}
	}
	newRustCompatibilityResolver = func() frameworkplan.CompatibilityBackendResolver {
		rustCount++
		return stubCompatibilityResolver{backendID: "rust"}
	}
	newJSCompatibilityResolver = func() frameworkplan.CompatibilityBackendResolver {
		jsCount++
		return stubCompatibilityResolver{backendID: "javascript"}
	}

	planner := ResolverFactory{Languages: WorkspaceLanguages{Go: true}}.CompatibilitySurfacePlanner()
	if planner == nil {
		t.Fatal("expected planner to be non-nil")
	}
	if goCount != 1 || pythonCount != 0 || rustCount != 0 || jsCount != 0 {
		t.Fatalf("unexpected constructor counts: go=%d python=%d rust=%d js=%d", goCount, pythonCount, rustCount, jsCount)
	}
}

func TestResolverFactoryFallbackConstructsAllResolvers(t *testing.T) {
	var verificationCount, compatibilityCount int
	originalGoVerification := newGoVerificationResolver
	originalPythonVerification := newPythonVerificationResolver
	originalRustVerification := newRustVerificationResolver
	originalJSVerification := newJSVerificationResolver
	originalGoCompatibility := newGoCompatibilityResolver
	originalPythonCompatibility := newPythonCompatibilityResolver
	originalRustCompatibility := newRustCompatibilityResolver
	originalJSCompatibility := newJSCompatibilityResolver
	t.Cleanup(func() {
		newGoVerificationResolver = originalGoVerification
		newPythonVerificationResolver = originalPythonVerification
		newRustVerificationResolver = originalRustVerification
		newJSVerificationResolver = originalJSVerification
		newGoCompatibilityResolver = originalGoCompatibility
		newPythonCompatibilityResolver = originalPythonCompatibility
		newRustCompatibilityResolver = originalRustCompatibility
		newJSCompatibilityResolver = originalJSCompatibility
	})

	newGoVerificationResolver = func() frameworkplan.VerificationBackendResolver {
		verificationCount++
		return stubVerificationResolver{backendID: "go"}
	}
	newPythonVerificationResolver = func() frameworkplan.VerificationBackendResolver {
		verificationCount++
		return stubVerificationResolver{backendID: "python"}
	}
	newRustVerificationResolver = func() frameworkplan.VerificationBackendResolver {
		verificationCount++
		return stubVerificationResolver{backendID: "rust"}
	}
	newJSVerificationResolver = func() frameworkplan.VerificationBackendResolver {
		verificationCount++
		return stubVerificationResolver{backendID: "javascript"}
	}
	newGoCompatibilityResolver = func() frameworkplan.CompatibilityBackendResolver {
		compatibilityCount++
		return stubCompatibilityResolver{backendID: "go"}
	}
	newPythonCompatibilityResolver = func() frameworkplan.CompatibilityBackendResolver {
		compatibilityCount++
		return stubCompatibilityResolver{backendID: "python"}
	}
	newRustCompatibilityResolver = func() frameworkplan.CompatibilityBackendResolver {
		compatibilityCount++
		return stubCompatibilityResolver{backendID: "rust"}
	}
	newJSCompatibilityResolver = func() frameworkplan.CompatibilityBackendResolver {
		compatibilityCount++
		return stubCompatibilityResolver{backendID: "javascript"}
	}

	if planner := (ResolverFactory{}).VerificationPlanner(); planner == nil {
		t.Fatal("expected verification planner to be non-nil")
	}
	if planner := (ResolverFactory{}).CompatibilitySurfacePlanner(); planner == nil {
		t.Fatal("expected compatibility planner to be non-nil")
	}
	if verificationCount != 4 {
		t.Fatalf("expected 4 verification resolver constructions, got %d", verificationCount)
	}
	if compatibilityCount != 4 {
		t.Fatalf("expected 4 compatibility resolver constructions, got %d", compatibilityCount)
	}
}
