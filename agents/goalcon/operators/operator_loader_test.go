package operators

import (
	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestNewOperatorRegistryFromConfig_NilConfig(t *testing.T) {
	registry := goalcon.NewOperatorRegistryFromConfig(nil)
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(registry.All()) == 0 {
		t.Fatal("expected default operators")
	}
}

func TestNewOperatorRegistryFromConfig_EmptyConfig(t *testing.T) {
	registry := goalcon.NewOperatorRegistryFromConfig(&goalcon.OperatorsConfigSection{})
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

	registry := goalcon.NewOperatorRegistryFromConfig(yamlConfig)
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
				"name":        "OpB",
				"description": "types.Operator B",
				"preconditions": []any{"stateA"},
				"effects":       []any{"stateB"},
			},
		},
	}

	registry := goalcon.NewOperatorRegistryFromConfig(mapConfig)
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

	registry := goalcon.NewOperatorRegistryFromConfig(jsonConfig)
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

	registry := goalcon.NewOperatorRegistryFromConfig(yamlConfig)
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
	section := &goalcon.OperatorsConfigSection{
		Operators: []goalcon.OperatorConfig{
			{Name: "Op1", Effects: []string{"e1"}},
		},
	}

	parsed, err := goalcon.ParseOperatorConfig(section)
	if err != nil {
		t.Fatalf("ParseOperatorConfig failed: %v", err)
	}
	if len(parsed.Operators) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(parsed.Operators))
	}
}

func TestValidateOperatorConfig_Valid(t *testing.T) {
	opConfig := goalcon.OperatorConfig{
		Name:          "ValidOp",
		Description:   "A valid operator",
		Preconditions: []string{"pre1"},
		Effects:       []string{"eff1", "eff2"},
	}

	err := goalcon.ValidateOperatorConfig(opConfig)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateOperatorConfig_NoName(t *testing.T) {
	opConfig := goalcon.OperatorConfig{
		Effects: []string{"eff1"},
	}

	err := goalcon.ValidateOperatorConfig(opConfig)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidateOperatorConfig_NoEffects(t *testing.T) {
	opConfig := goalcon.OperatorConfig{
		Name: "InvalidOp",
	}

	err := goalcon.ValidateOperatorConfig(opConfig)
	if err == nil {
		t.Fatal("expected error for missing effects")
	}
}

func TestValidateOperatorConfig_DuplicateEffects(t *testing.T) {
	opConfig := goalcon.OperatorConfig{
		Name:    "DupOp",
		Effects: []string{"eff1", "eff1"},
	}

	err := goalcon.ValidateOperatorConfig(opConfig)
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

	lib := goalcon.LoadOperatorLibraryFromConfig(libConfig, "mylib")
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
	lib := goalcon.LoadOperatorLibraryFromConfig(nil, "nonexistent")
	if lib != nil {
		t.Fatal("expected nil for nil config")
	}
}

func TestOperatorLibrary_ToRegistry(t *testing.T) {
	lib := &goalcon.OperatorLibrary{
		Name:    "testlib",
		Version: "1.0",
		Operators: []*goalcon.types.Operator{
			{
				Name:    "Op1",
				Effects: []goalcon.types.Predicate{"e1"},
			},
			{
				Name:    "Op2",
				Effects: []goalcon.types.Predicate{"e2"},
			},
		},
	}

	registry := lib.ToRegistry()
	if len(registry.All()) != 2 {
		t.Fatalf("expected 2 operators, got %d", len(registry.All()))
	}
}

func TestMergeRegistries_Single(t *testing.T) {
	reg1 := &goalcon.types.OperatorRegistry{}
	reg1.Register(goalcon.types.Operator{Name: "Op1", Effects: []goalcon.types.Predicate{"e1"}})

	merged := goalcon.MergeRegistries(reg1)
	if len(merged.All()) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(merged.All()))
	}
}

func TestMergeRegistries_Multiple(t *testing.T) {
	reg1 := &goalcon.types.OperatorRegistry{}
	reg1.Register(goalcon.types.Operator{Name: "Op1", Effects: []goalcon.types.Predicate{"e1"}})

	reg2 := &goalcon.types.OperatorRegistry{}
	reg2.Register(goalcon.types.Operator{Name: "Op2", Effects: []goalcon.types.Predicate{"e2"}})

	reg3 := &goalcon.types.OperatorRegistry{}
	reg3.Register(goalcon.types.Operator{Name: "Op3", Effects: []goalcon.types.Predicate{"e3"}})

	merged := goalcon.MergeRegistries(reg1, reg2, reg3)
	if len(merged.All()) != 3 {
		t.Fatalf("expected 3 operators, got %d", len(merged.All()))
	}
}

func TestMergeRegistries_WithNil(t *testing.T) {
	reg1 := &goalcon.types.OperatorRegistry{}
	reg1.Register(goalcon.types.Operator{Name: "Op1", Effects: []goalcon.types.Predicate{"e1"}})

	merged := goalcon.MergeRegistries(nil, reg1, nil)
	if len(merged.All()) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(merged.All()))
	}
}

func TestOperatorConfigValidator_ValidRegistry(t *testing.T) {
	registry := &goalcon.types.OperatorRegistry{}
	registry.Register(goalcon.types.Operator{Name: "Op1", Effects: []goalcon.types.Predicate{"e1"}})
	registry.Register(goalcon.types.Operator{
		Name:          "Op2",
		Preconditions: []goalcon.types.Predicate{"e1"},
		Effects:       []goalcon.types.Predicate{"e2"},
	})

	validator := &goalcon.OperatorConfigValidator{Strict: true}
	errors := validator.ValidateRegistry(registry)
	if len(errors) > 0 {
		t.Errorf("expected no errors, got %v", errors)
	}
}

func TestOperatorConfigValidator_DuplicateOperators(t *testing.T) {
	registry := &goalcon.types.OperatorRegistry{}
	registry.Register(goalcon.types.Operator{Name: "Op1", Effects: []goalcon.types.Predicate{"e1"}})
	registry.Register(goalcon.types.Operator{Name: "Op1", Effects: []goalcon.types.Predicate{"e2"}})

	validator := &goalcon.OperatorConfigValidator{Strict: true}
	errors := validator.ValidateRegistry(registry)
	if len(errors) == 0 {
		t.Fatal("expected error for duplicate operator")
	}
}

func TestOperatorConfigValidator_NoEffects(t *testing.T) {
	registry := &goalcon.types.OperatorRegistry{}
	registry.Register(goalcon.types.Operator{Name: "OpNoEffects"})

	validator := &goalcon.OperatorConfigValidator{Strict: true}
	errors := validator.ValidateRegistry(registry)
	if len(errors) == 0 {
		t.Fatal("expected error for operator with no effects")
	}
}

func TestGoalConAgent_CustomOperators(t *testing.T) {
	// Create agent with custom operators
	customRegistry := &goalcon.types.OperatorRegistry{}
	customRegistry.Register(goalcon.types.Operator{
		Name:    "CustomOp1",
		Effects: []goalcon.types.Predicate{"custom_effect"},
	})
	customRegistry.Register(goalcon.types.Operator{
		Name:          "CustomOp2",
		Preconditions: []goalcon.types.Predicate{"custom_effect"},
		Effects:       []goalcon.types.Predicate{"final_effect"},
	})

	agent := &goalcon.GoalConAgent{
		Operators: customRegistry,
	}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	ops := agent.Operators.All()
	if len(ops) != 2 {
		t.Fatalf("expected 2 custom operators, got %d", len(ops))
	}

	if ops[0].Name != "CustomOp1" {
		t.Errorf("expected CustomOp1, got %s", ops[0].Name)
	}
}

func TestGoalConAgent_DefaultOperators(t *testing.T) {
	// Create agent without custom operators
	agent := &goalcon.GoalConAgent{}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	ops := agent.Operators.All()
	if len(ops) == 0 {
		t.Fatal("expected default operators")
	}

	// Check that defaults include ReadFile, SearchCode, etc.
	found := false
	for _, op := range ops {
		if op.Name == "ReadFile" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected ReadFile in default operators")
	}
}

func TestParseOperatorConfig_InvalidJSON(t *testing.T) {
	// Binary data that's neither valid YAML nor JSON
	_, err := goalcon.ParseOperatorConfig(123) // Integer is not a valid format
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

	registry := goalcon.NewOperatorRegistryFromConfig(yamlConfig)
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

	lib1 := goalcon.LoadOperatorLibraryFromConfig(libConfig1, "lib1")
	lib2 := goalcon.LoadOperatorLibraryFromConfig(libConfig2, "lib2")

	if lib1 == nil || lib2 == nil {
		t.Fatal("expected both libraries to load")
	}

	if len(lib1.Operators) != 1 || len(lib2.Operators) != 1 {
		t.Fatal("expected 1 operator per library")
	}

	// Merge libraries
	merged := goalcon.MergeRegistries(lib1.ToRegistry(), lib2.ToRegistry())
	if len(merged.All()) != 2 {
		t.Fatalf("expected 2 operators after merge, got %d", len(merged.All()))
	}
}
