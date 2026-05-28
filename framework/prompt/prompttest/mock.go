// Package prompttest provides test helpers for the prompt registry.
package prompttest

import (
	"io/fs"
	"sync"

	"codeburg.org/lexbit/relurpify/framework/prompt"
)

// MockRegistry satisfies prompt.Registry for use in tests. Prompts are
// pre-seeded via With; providers are registered via WithProvider.
// All methods are safe for concurrent use.
type MockRegistry struct {
	mu        sync.RWMutex
	prompts   map[string]string // id → resolved content
	providers map[string]func(prompt.RuntimeContext) prompt.ContextChunk
	issues    map[string][]prompt.ValidationIssue
}

// New returns a new MockRegistry.
func New() *MockRegistry {
	return &MockRegistry{
		prompts:   make(map[string]string),
		providers: make(map[string]func(prompt.RuntimeContext) prompt.ContextChunk),
		issues:    make(map[string][]prompt.ValidationIssue),
	}
}

// With pre-loads a prompt id that resolves to the given content string.
func (m *MockRegistry) With(id, content string) *MockRegistry {
	m.mu.Lock()
	m.prompts[id] = content
	m.mu.Unlock()
	return m
}

// WithProvider registers a provider function by name.
func (m *MockRegistry) WithProvider(name string, fn func(prompt.RuntimeContext) prompt.ContextChunk) *MockRegistry {
	m.mu.Lock()
	m.providers[name] = fn
	m.mu.Unlock()
	return m
}

// WithIssue adds a validation issue for the given prompt id (for testing error paths).
func (m *MockRegistry) WithIssue(id string, iss prompt.ValidationIssue) *MockRegistry {
	m.mu.Lock()
	m.issues[id] = append(m.issues[id], iss)
	m.mu.Unlock()
	return m
}

// — Registry interface implementation —

func (m *MockRegistry) LoadDir(_ string) error                      { return nil }
func (m *MockRegistry) LoadFS(_ fs.FS, _ string) error              { return nil }
func (m *MockRegistry) ValidateProviders() []prompt.ValidationIssue { return nil }

func (m *MockRegistry) RegisterProvider(name string, _ prompt.ContextProvider) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.providers[name]; exists {
		return prompt.ErrAlreadyRegistered(name)
	}
	m.providers[name] = func(prompt.RuntimeContext) prompt.ContextChunk { return prompt.ContextChunk{} }
	return nil
}

func (m *MockRegistry) Get(id string) (*prompt.PromptConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.prompts[id]; ok {
		return &prompt.PromptConfig{ID: id}, true
	}
	return nil, false
}

func (m *MockRegistry) All() []*prompt.PromptConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*prompt.PromptConfig, 0, len(m.prompts))
	for id := range m.prompts {
		out = append(out, &prompt.PromptConfig{ID: id})
	}
	return out
}

func (m *MockRegistry) Filter(_ prompt.FilterOptions) []*prompt.PromptConfig {
	return m.All()
}

func (m *MockRegistry) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.prompts)
}

func (m *MockRegistry) Resolve(id string, ctx prompt.RuntimeContext) (string, error) {
	m.mu.RLock()
	content, ok := m.prompts[id]
	m.mu.RUnlock()
	if !ok {
		return "", &prompt.NotFoundError{ID: id}
	}
	return content, nil
}

func (m *MockRegistry) ResolveDryRun(id string, ctx prompt.RuntimeContext) (prompt.DryRunResult, error) {
	content, err := m.Resolve(id, ctx)
	if err != nil {
		return prompt.DryRunResult{}, err
	}
	return prompt.DryRunResult{Final: content}, nil
}

func (m *MockRegistry) DependsOn(_ string) ([]string, error)    { return nil, nil }
func (m *MockRegistry) DependentsOf(_ string) ([]string, error) { return nil, nil }

func (m *MockRegistry) PromptVariables(id string) (map[string]prompt.VariableDecl, error) {
	return nil, nil
}

func (m *MockRegistry) Validate(id string) []prompt.ValidationIssue {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.issues[id]
}

func (m *MockRegistry) ValidateAll() map[string][]prompt.ValidationIssue {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string][]prompt.ValidationIssue, len(m.issues))
	for id, iss := range m.issues {
		out[id] = append([]prompt.ValidationIssue{}, iss...)
	}
	return out
}
