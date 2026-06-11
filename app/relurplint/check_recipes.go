package main

import (
	"fmt"
	"os"
	"path/filepath"

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
		src, err := os.ReadFile(path)
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

	doc, err := thoughtrecipe.ParseSource(path, src)
	if err != nil {
		return []Diagnostic{{
			Check:    "recipes",
			Code:     "recipes.parse",
			Severity: SeverityError,
			Loc:      SourceLoc{File: rel, Line: extractLine(err.Error())},
			Message:  err.Error(),
		}}
	}

	plan, err := thoughtrecipe.LowerDocument(doc)
	if err != nil {
		return []Diagnostic{{
			Check:    "recipes",
			Code:     "recipes.lower",
			Severity: SeverityError,
			Loc:      SourceLoc{File: rel, Line: extractLine(err.Error())},
			Message:  err.Error(),
		}}
	}

	if err := plan.ThoughtRecipe.Validate(); err != nil {
		return []Diagnostic{{
			Check:    "recipes",
			Code:     "recipes.validate",
			Severity: SeverityError,
			Loc:      SourceLoc{File: rel},
			Message:  err.Error(),
		}}
	}

	return nil
}

func relPath(workspace, path string) string {
	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		return path
	}
	return rel
}
