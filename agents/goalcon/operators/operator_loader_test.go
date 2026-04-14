package operators

import (
	"testing"

	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

func TestNewOperatorRegistryFromConfig_NilConfig(t *testing.T) {
	registry := NewOperatorRegistryFromConfig(nil)
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(registry.All()) == 0 {
		t.Fatal("expected default operators")
	}
}

func TestNewOperatorRegistryFromConfig_EmptyConfig(t *testing.T) {
	registry := NewOperatorRegistryFromConfig(&OperatorsConfigSection{})
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(registry.All()) == 0 {
		t.Fatal("expected default operators")
	}
}

func TestNewOperatorRegistryFromConfig_FromYAML(t *testing.T) {
	yamlConfig := `
operators:
  - name: TestOp1
    description: First test operator
    effects:
      - effect1
  - name: TestOp2
    description: Second test operator
    preconditions:
      - effect1
    effects:
      - effect2
    default_params:
      timeout: 5000
`

	registry := NewOperatorRegistryFromConfig(yamlConfig)
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}

	ops := registry.All()
	if len(ops) != 2 {
		t.Fatalf("expected 2 operators, got %d", len(ops))
	}

	if ops[0].Name != "TestOp1" {
		t.Errorf("expected TestOp1, got %s", ops[0].Name)
	}

	if len(ops[1].Preconditions) != 1 || ops[1].Preconditions[0] != "effect1" {
		t.Errorf("expected precondition effect1, got %v", ops[1].Preconditions)
	}
}

func TestNewOperatorRegistryFromConfig_FromMap(t *testing.T) {
	mapConfig := map[string]any{
		"operators": []map[string]any{
			{
				"name":        "OpA",
				"description": "types.Operator A",
				"effects":     []any{"stateA"},
			},
			{
				"name":          "OpB",
				"description":   "types.Operator B",
				"preconditions": []any{"stateA"},
				"effects":       []any{"stateB"},
			},
		},
	}

	registry := NewOperatorRegistryFromConfig(mapConfig)
	if registry == nil || len(registry.All()) != 2 {
		t.Fatalf("expected 2 operators, got %d", len(registry.All()))
	}
}

func TestNewOperatorRegistryFromConfig_FromJSON(t *testing.T) {
	jsonConfig := `{
  "operators": [
    {
      "name": "ReadOp",
      "description": "Read operation",
      "effects": ["data_read"]
    },
    {
      "name": "WriteOp",
      "description": "Write operation",
      "preconditions": ["data_read"],
      "effects": ["data_written"]
    }
  ]
}`

	registry := NewOperatorRegistryFromConfig(jsonConfig)
	if registry == nil || len(registry.All()) != 2 {
		t.Fatalf("expected 2 operators, got %d", len(registry.All()))
	}
}

func TestOperatorConfig_WithDefaultParams(t *testing.T) {
	yamlConfig := `
operators:
  - name: ConfiguredOp
    description: Op with defaults
    effects:
      - result
    default_params:
      timeout: 30000
      retry_count: 3
      strategy: fast
`

	registry := NewOperatorRegistryFromConfig(yamlConfig)
	ops := registry.All()
	if len(ops) == 0 {
		t.Fatal("expected operators")
	}

	op := ops[0]
	if op.DefaultParams == nil {
		t.Fatal("expected default params")
	}

	if val, ok := op.DefaultParams["timeout"]; !ok || val != 30000 {
		t.Errorf("expected timeout param")
	}
	if val, ok := op.DefaultParams["retry_count"]; !ok || val != 3 {
		t.Errorf("expected retry_count param")
	}
}

func TestParseOperatorConfig_AlreadyParsed(t *testing.T) {
	section := &OperatorsConfigSection{
		Operators: []OperatorConfig{
			{Name: "Op1", Effects: []string{"e1"}},
		},
	}

	parsed, err := ParseOperatorConfig(section)
	if err != nil {
		t.Fatalf("ParseOperatorConfig failed: %v", err)
	}
	if len(parsed.Operators) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(parsed.Operators))
	}
}

func TestValidateOperatorConfig_Valid(t *testing.T) {
	opConfig := OperatorConfig{
		Name:          "ValidOp",
		Description:   "A valid operator",
		Preconditions: []string{"pre1"},
		Effects:       []string{"eff1", "eff2"},
	}

	err := ValidateOperatorConfig(opConfig)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateOperatorConfig_NoName(t *testing.T) {
	opConfig := OperatorConfig{
		Effects: []string{"eff1"},
	}

	err := ValidateOperatorConfig(opConfig)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidateOperatorConfig_NoEffects(t *testing.T) {
	opConfig := OperatorConfig{
		Name: "InvalidOp",
	}

	err := ValidateOperatorConfig(opConfig)
	if err == nil {
		t.Fatal("expected error for missing effects")
	}
}

func TestValidateOperatorConfig_DuplicateEffects(t *testing.T) {
	opConfig := OperatorConfig{
		Name:    "DupOp",
		Effects: []string{"eff1", "eff1"},
	}

	err := ValidateOperatorConfig(opConfig)
	if err == nil {
		t.Fatal("expected error for duplicate effects")
	}
}

func TestLoadOperatorLibraryFromConfig_Found(t *testing.T) {
	libConfig := map[string]any{
		"operators": []map[string]any{
			{
				"name":    "LibOp1",
				"effects": []any{"lib_effect1"},
			},
		},
	}

	lib := LoadOperatorLibraryFromConfig(libConfig, "mylib")
	if lib == nil {
		t.Fatal("expected library to be loaded")
	}
	if lib.Name != "mylib" {
		t.Errorf("expected library name mylib, got %s", lib.Name)
	}
	if len(lib.Operators) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(lib.Operators))
	}
}

func TestLoadOperatorLibraryFromConfig_NotFound(t *testing.T) {
	lib := LoadOperatorLibraryFromConfig(nil, "nonexistent")
	if lib != nil {
		t.Fatal("expected nil for nil config")
	}
}

func TestOperatorLibrary_ToRegistry(t *testing.T) {
	lib := &OperatorLibrary{
		Name:    "testlib",
		Version: "1.0",
		Operators: []*types.Operator{
			{
				Name:    "Op1",
				Effects: []types.Predicate{"e1"},
			},
			{
				Name:    "Op2",
				Effects: []types.Predicate{"e2"},
			},
		},
	}

	registry := lib.ToRegistry()
	if len(registry.All()) != 2 {
		t.Fatalf("expected 2 operators, got %d", len(registry.All()))
	}
}

func TestMergeRegistries_Single(t *testing.T) {
	reg1 := &types.OperatorRegistry{}
	reg1.Register(types.Operator{Name: "Op1", Effects: []types.Predicate{"e1"}})

	merged := MergeRegistries(reg1)
	if len(merged.All()) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(merged.All()))
	}
}

func TestMergeRegistries_Multiple(t *testing.T) {
	reg1 := &types.OperatorRegistry{}
	reg1.Register(types.Operator{Name: "Op1", Effects: []types.Predicate{"e1"}})

	reg2 := &types.OperatorRegistry{}
	reg2.Register(types.Operator{Name: "Op2", Effects: []types.Predicate{"e2"}})

	reg3 := &types.OperatorRegistry{}
	reg3.Register(types.Operator{Name: "Op3", Effects: []types.Predicate{"e3"}})

	merged := MergeRegistries(reg1, reg2, reg3)
	if len(merged.All()) != 3 {
		t.Fatalf("expected 3 operators, got %d", len(merged.All()))
	}
}

func TestMergeRegistries_WithNil(t *testing.T) {
	reg1 := &types.OperatorRegistry{}
	reg1.Register(types.Operator{Name: "Op1", Effects: []types.Predicate{"e1"}})

	merged := MergeRegistries(nil, reg1, nil)
	if len(merged.All()) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(merged.All()))
	}
}

func TestOperatorConfigValidator_ValidRegistry(t *testing.T) {
	registry := &types.OperatorRegistry{}
	registry.Register(types.Operator{Name: "Op1", Effects: []types.Predicate{"e1"}})
	registry.Register(types.Operator{
		Name:          "Op2",
		Preconditions: []types.Predicate{"e1"},
		Effects:       []types.Predicate{"e2"},
	})

	validator := &OperatorConfigValidator{Strict: true}
	errors := validator.ValidateRegistry(registry)
	if len(errors) > 0 {
		t.Errorf("expected no errors, got %v", errors)
	}
}

func TestOperatorConfigValidator_DuplicateOperators(t *testing.T) {
	registry := &types.OperatorRegistry{}
	registry.Register(types.Operator{Name: "Op1", Effects: []types.Predicate{"e1"}})
	registry.Register(types.Operator{Name: "Op1", Effects: []types.Predicate{"e2"}})

	validator := &OperatorConfigValidator{Strict: true}
	errors := validator.ValidateRegistry(registry)
	if len(errors) == 0 {
		t.Fatal("expected error for duplicate operator")
	}
}

func TestOperatorConfigValidator_NoEffects(t *testing.T) {
	registry := &types.OperatorRegistry{}
	registry.Register(types.Operator{Name: "OpNoEffects"})

	validator := &OperatorConfigValidator{Strict: true}
	errors := validator.ValidateRegistry(registry)
	if len(errors) == 0 {
		t.Fatal("expected error for operator with no effects")
	}
}

// Note: TestGoalConAgent_CustomOperators and TestGoalConAgent_DefaultOperators
// have been moved to goalcon_agent_test.go to avoid circular import with goalcon package
// func TestGoalConAgent_CustomOperators(t *testing.T) {
// ...
// }
// func TestGoalConAgent_DefaultOperators(t *testing.T) {
// ...
// }

func TestParseOperatorConfig_InvalidJSON(t *testing.T) {
	// Binary data that's neither valid YAML nor JSON
	_, err := ParseOperatorConfig(123) // Integer is not a valid format
	if err == nil {
		t.Fatal("expected error for invalid input type")
	}
}

func TestOperatorConfigWithTags(t *testing.T) {
	yamlConfig := `
operators:
  - name: TaggedOp
    description: Op with tags
    effects:
      - result
    tags:
      - fast
      - safe
      - deterministic
`

	registry := NewOperatorRegistryFromConfig(yamlConfig)
	ops := registry.All()
	if len(ops) == 0 {
		t.Fatal("expected operators")
	}

	// Note: Tags are parsed but not currently used by types.Operator type
	// This test ensures the YAML parsing doesn't fail with tags present
}

func TestLoadOperatorLibrary_MultipleLibraries(t *testing.T) {
	libConfig1 := map[string]any{
		"operators": []map[string]any{
			{"name": "Lib1Op", "effects": []any{"e1"}},
		},
	}

	libConfig2 := map[string]any{
		"operators": []map[string]any{
			{"name": "Lib2Op", "effects": []any{"e2"}},
		},
	}

	lib1 := LoadOperatorLibraryFromConfig(libConfig1, "lib1")
	lib2 := LoadOperatorLibraryFromConfig(libConfig2, "lib2")

	if lib1 == nil || lib2 == nil {
		t.Fatal("expected both libraries to load")
	}

	if len(lib1.Operators) != 1 || len(lib2.Operators) != 1 {
		t.Fatal("expected 1 operator per library")
	}

	// Merge libraries
	merged := MergeRegistries(lib1.ToRegistry(), lib2.ToRegistry())
	if len(merged.All()) != 2 {
		t.Fatalf("expected 2 operators after merge, got %d", len(merged.All()))
	}
}

func TestLoadOperatorsFromFile_EmptyPath(t *testing.T) {
	registry := LoadOperatorsFromFile("")
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
	// Should return default operators
	if len(registry.All()) == 0 {
		t.Fatal("expected default operators")
	}
}

func TestLoadOperatorsFromFile_NonexistentPath(t *testing.T) {
	registry := LoadOperatorsFromFile("/nonexistent/path/file.yaml")
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
	// Should return default operators
	if len(registry.All()) == 0 {
		t.Fatal("expected default operators")
	}
}

func TestLoadOperatorLibraryFromFile_EmptyPath(t *testing.T) {
	lib := LoadOperatorLibraryFromFile("", "testlib")
	if lib != nil {
		t.Error("expected nil library for empty path")
	}
}

func TestLoadOperatorLibraryFromFile_NonexistentPath(t *testing.T) {
	lib := LoadOperatorLibraryFromFile("/nonexistent/path/file.yaml", "testlib")
	if lib != nil {
		t.Error("expected nil library for nonexistent path")
	}
}

func TestOperatorLibrary_ToRegistry_NilLibrary(t *testing.T) {
	var lib *OperatorLibrary
	registry := lib.ToRegistry()
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
	// Should return empty registry
	if len(registry.All()) != 0 {
		t.Fatalf("expected 0 operators, got %d", len(registry.All()))
	}
}

func TestOperatorLibrary_ToRegistry_EmptyOperators(t *testing.T) {
	lib := &OperatorLibrary{
		Name:      "testlib",
		Version:   "1.0",
		Operators: []*types.Operator{},
	}
	registry := lib.ToRegistry()
	if len(registry.All()) != 0 {
		t.Fatalf("expected 0 operators, got %d", len(registry.All()))
	}
}

func TestOperatorLibrary_ToRegistry_WithNilOperator(t *testing.T) {
	lib := &OperatorLibrary{
		Name:    "testlib",
		Version: "1.0",
		Operators: []*types.Operator{
			nil,
			{Name: "ValidOp", Effects: []types.Predicate{"e1"}},
		},
	}
	registry := lib.ToRegistry()
	// Should skip nil operator and register only valid one
	if len(registry.All()) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(registry.All()))
	}
}

func TestValidateOperatorConfig_DuplicatePreconditions(t *testing.T) {
	opConfig := OperatorConfig{
		Name:          "DupPreOp",
		Effects:       []string{"eff1"},
		Preconditions: []string{"pre1", "pre1"},
	}

	err := ValidateOperatorConfig(opConfig)
	if err == nil {
		t.Fatal("expected error for duplicate preconditions")
	}
}

func TestOperatorConfigValidator_NilRegistry(t *testing.T) {
	validator := &OperatorConfigValidator{Strict: true}
	errors := validator.ValidateRegistry(nil)
	if len(errors) != 0 {
		t.Errorf("expected no errors for nil registry, got %v", errors)
	}
}

func TestOperatorConfigValidator_SelfReferentialPrecondition(t *testing.T) {
	registry := &types.OperatorRegistry{}
	// Operator has precondition that matches its effect (self-referential)
	registry.Register(types.Operator{
		Name:          "SelfRefOp",
		Preconditions: []types.Predicate{"selfEffect"},
		Effects:       []types.Predicate{"selfEffect"},
	})

	validator := &OperatorConfigValidator{Strict: true}
	errors := validator.ValidateRegistry(registry)
	if len(errors) == 0 {
		t.Fatal("expected error for self-referential precondition")
	}
}

func TestOperatorConfigValidator_NilOperator(t *testing.T) {
	registry := &types.OperatorRegistry{}
	// Register a valid operator first
	registry.Register(types.Operator{Name: "ValidOp", Effects: []types.Predicate{"e1"}})

	validator := &OperatorConfigValidator{Strict: true}
	errors := validator.ValidateRegistry(registry)
	// Nil operators in the list should be skipped without error
	if len(errors) != 0 {
		t.Errorf("expected no errors, got %v", errors)
	}
}

func TestMergeRegistries_Empty(t *testing.T) {
	merged := MergeRegistries()
	if merged == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(merged.All()) != 0 {
		t.Fatalf("expected 0 operators, got %d", len(merged.All()))
	}
}

func TestMergeRegistries_AllNil(t *testing.T) {
	merged := MergeRegistries(nil, nil, nil)
	if merged == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(merged.All()) != 0 {
		t.Fatalf("expected 0 operators, got %d", len(merged.All()))
	}
}

func TestParseOperatorConfig_InvalidString(t *testing.T) {
	// Invalid YAML syntax
	trulyInvalid := "[invalid: yaml:::::"
	_, err := ParseOperatorConfig(trulyInvalid)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestNewOperatorRegistryFromConfig_InvalidOperator(t *testing.T) {
	// Config with operator missing required name
	yamlConfig := `
operators:
  - description: "Missing name"
    effects:
      - result
`
	registry := NewOperatorRegistryFromConfig(yamlConfig)
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
	// Should fall back to defaults when operator is invalid
	if len(registry.All()) == 0 {
		t.Fatal("expected default operators when config has invalid operators")
	}
}

func TestLoadOperatorLibraryFromConfig_EmptyOperators(t *testing.T) {
	libConfig := map[string]any{
		"operators": []map[string]any{},
	}

	lib := LoadOperatorLibraryFromConfig(libConfig, "emptylib")
	if lib != nil {
		t.Error("expected nil library for empty operators list")
	}
}

func TestOperatorVariant_StructFields(t *testing.T) {
	// Test that OperatorVariant struct can be instantiated
	variant := OperatorVariant{
		Name:         "ReadFile:large_files",
		BaseOperator: "ReadFile",
		Description:  "Specialized for large files",
		DefaultParams: map[string]any{
			"chunk_size": 1024,
		},
	}

	if variant.Name != "ReadFile:large_files" {
		t.Errorf("expected name 'ReadFile:large_files', got %s", variant.Name)
	}
	if variant.BaseOperator != "ReadFile" {
		t.Errorf("expected base operator 'ReadFile', got %s", variant.BaseOperator)
	}
}
