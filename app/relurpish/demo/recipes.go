package demo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	templates "codeburg.org/lexbit/relurpify/userconfig/templates/embedfs"
)

func (s *Session) loadRecipes() error {
	s.recipes = make(map[string]tui.RecipeFile)
	const root = "workspace/euclo"
	entries, err := fs.ReadDir(templates.DefaultFS(), root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".erpe" {
			continue
		}
		data, err := fs.ReadFile(templates.DefaultFS(), path.Join(root, entry.Name()))
		if err != nil {
			return err
		}
		document, err := thoughtrecipe.ParseSource(entry.Name(), string(data))
		if err != nil {
			return err
		}
		s.recipes[document.Name] = tui.RecipeFile{ID: document.Name, Path: path.Join("relurpify_cfg/euclo", entry.Name()), Revision: "1", Source: string(data)}
	}
	return nil
}

// DEMO ONLY: Lists mutable in-memory copies of real bundled recipe sources.
// LIVE CONTRACT: RecipeWorkspace.ListRecipes/OpenRecipe.
// REPLACEMENT: Supply the live recipe workspace's source/revision records.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) ListRecipes(ctx context.Context) ([]tui.RecipeFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	files := make([]tui.RecipeFile, 0, len(s.recipes))
	for _, file := range s.recipes {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ID < files[j].ID })
	return files, nil
}

// DEMO ONLY: Retrieves the scenario's current saved source revision.
// LIVE CONTRACT: RecipeWorkspace.OpenRecipe.
// REPLACEMENT: Supply live source identity, content, and revision through the port.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) OpenRecipe(ctx context.Context, id string) (tui.RecipeFile, error) {
	if err := ctx.Err(); err != nil {
		return tui.RecipeFile{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, ok := s.recipes[id]
	if !ok {
		return tui.RecipeFile{}, fmt.Errorf("recipe %q is not in the demo workspace", id)
	}
	return file, nil
}

// DEMO ONLY: Adds illustrative prompt records to the actual installed strategy
// contract catalog. Prompt text is explicitly preview data, never actual-call evidence.
// LIVE CONTRACT: RecipeWorkspace.RecipeCatalog.
// REPLACEMENT: Supply the live prompt registry's references, versions and previews.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) RecipeCatalog(ctx context.Context) (tui.RecipeCatalog, error) {
	if err := ctx.Err(); err != nil {
		return tui.RecipeCatalog{}, err
	}
	var catalog tui.RecipeCatalog
	for _, contract := range paradigm.Registry.All() {
		catalog.Strategies = append(catalog.Strategies, tui.RecipeStrategy{Name: contract.Paradigm, Shape: string(contract.Shape)})
	}
	catalog.Prompts = []tui.RecipePrompt{
		{ID: "demo.prompt.repair", Version: "demo-v1", Origin: "DEMO scenario catalog", Text: "Inspect the requested behavior and its test. Make the smallest repair, run the focused test runner, and report observed verification separately from claims."},
		{ID: "demo.prompt.review", Version: "demo-v1", Origin: "DEMO scenario catalog", Text: "Review the selected changes. Explain concrete defects with file and line evidence. Distinguish observations from hypotheses and do not modify workspace files."},
	}
	return catalog, nil
}

// DEMO ONLY: Saves a source revision to scenario memory after real parser and
// installed strategy-contract validation. Import/tool resolution is deferred.
// LIVE CONTRACT: RecipeWorkspace.SaveRecipe, source revision compare-and-swap.
// REPLACEMENT: Supply the governed, lossless live source owner's save result.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) SaveRecipe(ctx context.Context, file tui.RecipeFile) (tui.RecipeFile, error) {
	if err := ctx.Err(); err != nil {
		return tui.RecipeFile{}, err
	}
	if len(file.Source) > 262144 {
		return tui.RecipeFile{}, errors.New("recipe source exceeds 256 KiB")
	}
	document, err := thoughtrecipe.ParseSource(file.Path, file.Source)
	if err != nil {
		return tui.RecipeFile{}, err
	}
	if document.Name != file.ID {
		return tui.RecipeFile{}, errors.New("renaming recipe identity requires a separate source operation")
	}
	if diagnostics := thoughtrecipe.ValidateAgainstContracts(document, nil); len(diagnostics) > 0 {
		return tui.RecipeFile{}, diagnostics[0]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.recipes[file.ID]
	if !ok || existing.Revision != file.Revision {
		return tui.RecipeFile{}, errors.New("saved source changed; local draft retained, reopen after reviewing the conflict")
	}
	revision, err := strconv.ParseUint(existing.Revision, 10, 64)
	if err != nil {
		return tui.RecipeFile{}, err
	}
	file.Revision = strconv.FormatUint(revision+1, 10)
	s.recipes[file.ID] = file
	return file, nil
}

// DEMO ONLY: Triggers exactly the saved scenario source and its parsed run graph.
// The request input is separate from ordinary intent/chat recipe selection.
// LIVE CONTRACT: RecipeWorkspace.TriggerRecipe, saved-file admission.
// REPLACEMENT: Supply real file identity/revision capture and execution admission.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) TriggerRecipe(ctx context.Context, file tui.RecipeFile, input string) (tui.FrontendSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return tui.FrontendSnapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, ok := s.recipes[file.ID]
	if !ok || saved.Revision != file.Revision || saved.Source != file.Source {
		return clone(s.state), errors.New("saved file revision changed; trigger was not admitted")
	}
	if s.state.Running {
		return clone(s.state), errors.New("a demo request is already running")
	}
	if strings.TrimSpace(input) == "" {
		return clone(s.state), errors.New("recipe request input is required")
	}
	document, err := thoughtrecipe.ParseSource(saved.Path, saved.Source)
	if err != nil {
		return clone(s.state), err
	}
	agents := make(map[string]string)
	for _, decl := range document.Declarations {
		if agent, ok := decl.(*thoughtrecipe.AgentDecl); ok {
			agents[agent.Name.Value] = agent.AgentType.Value
		}
	}
	var steps []tui.CircuitStep
	for _, decl := range document.Declarations {
		run, ok := decl.(*thoughtrecipe.RunDecl)
		if !ok {
			continue
		}
		step := tui.CircuitStep{ID: fmt.Sprintf("%s-%d", run.Agent.Value, len(steps)+1), Strategy: agents[run.Agent.Value]}
		for _, item := range run.Items {
			if goal, ok := item.(*thoughtrecipe.GoalClause); ok {
				step.Goal = goal.Text.Value
			}
		}
		steps = append(steps, step)
	}
	if len(steps) == 0 {
		return clone(s.state), errors.New("this demo executor supports top-level run declarations; this file has none")
	}
	s.beginLocked("Trigger saved file " + saved.Path + " @" + saved.Revision + ": " + input)
	steps[0].Status = "active"
	s.state.Circuit = tui.CircuitSnapshot{RecipeID: saved.ID, Name: saved.ID, Steps: steps, Notice: "DEMO ONLY: parsed saved source revision " + saved.Revision + ". Strategy execution, prompt imports and tools are simulated, not runtime verified."}
	s.publishLocked()
	return clone(s.state), nil
}
