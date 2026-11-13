package arch

import (
	"slices"
	"testing"
)

func TestRelurpishV2RejectsProhibitedConstructs(t *testing.T) {
	for name, source := range map[string]string{
		"alias":       "package p; type ID = string",
		"test_api":    "package p; func SetFocusForTest() {}",
		"reflection":  "package p; func f(v V) { v.FieldByName(\"anything\") }",
		"renamed_os":  "package p; import disk \"os\"; func f(){ disk.WriteFile(\"x\",nil,0600) }",
		"renamed_env": "package p; import system \"os\"; func f(){ system.LookupEnv(\"TOKEN\") }",
		"dot_import":  "package p; import . \"os\"; func f(){ WriteFile(\"x\",nil,0600) }",
	} {
		t.Run(name, func(t *testing.T) {
			violations, err := CheckRelurpishV2Source("app/relurpish/euclotui/view.go", []byte(source))
			if err != nil || len(violations) == 0 {
				t.Fatalf("gate did not reject %s: %v %v", name, violations, err)
			}
		})
	}
}

func TestRelurpishV2TestInventoryRejectsMissingTests(t *testing.T) {
	missing := MissingRelurpishV2Tests("ok example/package 0.1s [no tests to run]\n")
	if !slices.Contains(missing, "TestV2PlainTextEverySurface") || !slices.Contains(missing, "TestV2CircuitSelectionSurvivesProgress") {
		t.Fatalf("missing test gate did not fail: %v", missing)
	}
	partial := MissingRelurpishV2Tests("TestV2PlainTextEverySurface\n")
	if len(partial) != len(missing)-1 || slices.Contains(partial, "TestV2PlainTextEverySurface") {
		t.Fatal("partial inventory accepted")
	}
}

func TestRelurpishV2AcceptsOwnedVocabularyAndReadOnlyView(t *testing.T) {
	violations, err := CheckRelurpishV2Source("app/relurpish/euclotui/view.go", []byte("package p; import disk \"os\"; type ID string; func f(){ disk.ReadFile(\"x\") }"))
	if err != nil || len(violations) != 0 {
		t.Fatalf("valid source rejected: %v %v", violations, err)
	}
}
