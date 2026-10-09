package main

import (
	"fmt"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

type recipesCheck struct{}

func init() {
	registerCheck(recipesCheck{})
}

func (c recipesCheck) Name() string { return "recipes" }

func (c recipesCheck) Run(workspace string) []Diagnostic {
	recipesDir := filepath.Join(workspace, thoughtrecipe.ThoughtRecipeSourceRoot)
	entries, err := os.ReadDir(recipesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []Diagnostic{{
			Check:    "recipes",
			Code:     "recipes.discover",
			Severity: SeverityError,
			Message:  fmt.Sprintf("read recipe directory: %v", err),
		}}
	}

	var diags []Diagnostic
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if !thoughtrecipe.IsAcceptedThoughtRecipeExtension(ext) {
			continue
		}

		path := filepath.Join(recipesDir, entry.Name())
		src, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			diags = append(diags, Diagnostic{
				Check:    "recipes",
				Code:     "recipes.read",
				Severity: SeverityError,
				Loc:      SourceLoc{File: relPath(workspace, path)},
				Message:  fmt.Sprintf("read recipe file: %v", err),
			})
			continue
		}

		diags = append(diags, validateRecipe(workspace, path, string(src))...)
	}
	return diags
}

func validateRecipe(workspace, path, src string) []Diagnostic {
	rel := relPath(workspace, path)
	var diags []Diagnostic

	doc, err := thoughtrecipe.ParseSource(path, src)
	if err != nil {
		diags = append(diags, Diagnostic{
			Check:    "recipes",
			Code:     "recipes.parse",
			Severity: SeverityError,
			Loc:      SourceLoc{File: rel, Line: extractLine(err.Error())},
			Message:  err.Error(),
		})
		return diags
	}

	// Contract-aware check (Wave 2 Phase 3): every recipe is validated
	// against the paradigm contract registry so a retired directive or an
	// unknown paradigm surfaces at lint time with the same errors the loader
	// would surface at boot.
	for _, contractErr := range thoughtrecipe.ValidateAgainstContracts(doc, paradigm.Registry) {
		diags = append(diags, contractDiagnostic(rel, contractErr))
	}

	plan, err := thoughtrecipe.LowerDocument(doc)
	if err != nil {
		return append(diags, Diagnostic{
			Check:    "recipes",
			Code:     "recipes.lower",
			Severity: SeverityError,
			Loc:      SourceLoc{File: rel, Line: extractLine(err.Error())},
			Message:  err.Error(),
		})
	}

	if err := thoughtrecipe.ValidatePlanContracts(plan, paradigm.Registry); err != nil {
		diags = append(diags, Diagnostic{
			Check:    "recipes",
			Code:     "recipes.contract",
			Severity: SeverityError,
			Loc:      SourceLoc{File: rel},
			Message:  err.Error(),
		})
	}

	if err := plan.ThoughtRecipe.Validate(); err != nil {
		diags = append(diags, Diagnostic{
			Check:    "recipes",
			Code:     "recipes.validate",
			Severity: SeverityError,
			Loc:      SourceLoc{File: rel},
			Message:  err.Error(),
		})
	}

	return diags
}

func contractDiagnostic(rel string, contractErr error) Diagnostic {
	return Diagnostic{
		Check:    "recipes",
		Code:     "recipes.contract",
		Severity: SeverityError,
		Loc:      SourceLoc{File: rel, Line: extractLine(contractErr.Error())},
		Message:  contractErr.Error(),
	}
}

func relPath(workspace, path string) string {
	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		return path
	}
	return rel
}
