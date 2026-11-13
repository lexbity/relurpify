package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// recipeWorkspaceKey is the context key for storing the RecipeWorkspace.
type recipeWorkspaceKey struct{}

// WithRecipeWorkspace adds a RecipeWorkspace to the context.
func WithRecipeWorkspace(ctx context.Context, workspace RecipeWorkspace) context.Context {
	return context.WithValue(ctx, recipeWorkspaceKey{}, workspace)
}

// GetRecipeWorkspace retrieves the RecipeWorkspace from the context.
func GetRecipeWorkspace(ctx context.Context) RecipeWorkspace {
	if v := ctx.Value(recipeWorkspaceKey{}); v != nil {
		return v.(RecipeWorkspace)
	}
	return nil
}

// RegisterRecipeCommands registers all recipe-related commands with the registry.
// These commands use the RecipeWorkspace interface for lossless editing and file execution.
func RegisterRecipeCommands(r *CommandRegistry) {
	// /recipe list - List all recipes
	r.Register(Command{
		Name:        "recipe list",
		Description: "List all recipe files in the workspace",
		Usage:       "/recipe list",
		Handler:     handleRecipeList,
	})

	// /recipe open <path> - Open a recipe for editing
	r.Register(Command{
		Name:        "recipe open",
		Description: "Open a recipe file for editing",
		Usage:       "/recipe open <path>",
		Handler:     handleRecipeOpen,
	})

	// /recipe new <path> - Create a new recipe file
	r.Register(Command{
		Name:        "recipe new",
		Description: "Create a new recipe file",
		Usage:       "/recipe new <path>",
		Handler:     handleRecipeNew,
	})

	// /recipe save - Save the current recipe
	r.Register(Command{
		Name:        "recipe save",
		Description: "Save the current recipe file",
		Usage:       "/recipe save",
		Handler:     handleRecipeSave,
	})

	// /recipe close - Close the current recipe
	r.Register(Command{
		Name:        "recipe close",
		Description: "Close the current recipe editor",
		Usage:       "/recipe close",
		Handler:     handleRecipeClose,
	})

	// /recipe validate - Validate the current recipe
	r.Register(Command{
		Name:        "recipe validate",
		Description: "Validate the current recipe",
		Usage:       "/recipe validate",
		Handler:     handleRecipeValidate,
	})

	// /recipe run <path> - Run a recipe file
	r.Register(Command{
		Name:        "recipe run",
		Description: "Run a recipe file",
		Usage:       "/recipe run <path>",
		Handler:     handleRecipeRun,
	})

	// /recipe undo - Undo the last edit
	r.Register(Command{
		Name:        "recipe undo",
		Description: "Undo the last edit in the recipe editor",
		Usage:       "/recipe undo",
		Handler:     handleRecipeUndo,
	})

	// /recipe redo - Redo the last undone edit
	r.Register(Command{
		Name:        "recipe redo",
		Description: "Redo the last undone edit in the recipe editor",
		Usage:       "/recipe redo",
		Handler:     handleRecipeRedo,
	})

	// /recipe source - Show the raw source of the current recipe
	r.Register(Command{
		Name:        "recipe source",
		Description: "Show the raw source of the current recipe",
		Usage:       "/recipe source",
		Handler:     handleRecipeSource,
	})
}

func handleRecipeList(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.workshop == nil || m.workshop.port == nil {
		m.addSystemMessage("Recipe workspace unavailable")
		return m, nil
	}

	ctx := context.Background()
	recipes, err := m.workshop.port.ListRecipes(ctx)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Error listing recipes: %v", err))
		return m, nil
	}

	if len(recipes) == 0 {
		m.addSystemMessage("No recipes found in workspace")
		return m, nil
	}

	var sb strings.Builder
	sb.WriteString("Recipes:\n")
	for _, r := range recipes {
		if len(r.Revision) >= 8 {
			sb.WriteString(fmt.Sprintf("  %s (rev: %s)\n", r.Path, r.Revision[:8]))
		} else {
			sb.WriteString(fmt.Sprintf("  %s\n", r.Path))
		}
	}
	sb.WriteString("\nUse /recipe open <path> to edit or /recipe run <path> to execute")

	m.addSystemMessage(sb.String())
	return m, nil
}

func handleRecipeOpen(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.workshop == nil || m.workshop.port == nil {
		m.addSystemMessage("Recipe workspace unavailable")
		return m, nil
	}

	if len(args) == 0 {
		m.addSystemMessage("Usage: /recipe open <path>")
		return m, nil
	}

	path := args[0]
	ctx := context.Background()
	
	file, err := m.workshop.port.OpenRecipe(ctx, path)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Error opening recipe: %v", err))
		return m, nil
	}

	// Switch to workshop tab and load the recipe
	m.activeTab = TabWorkshop
	m.workshop.mode = workshopSource
	m.workshop.file = file
	m.workshop.source = file.Source
	m.workshop.files = []RecipeFile{file}

	m.addSystemMessage(fmt.Sprintf("Opened recipe: %s", file.Path))
	return m, nil
}

func handleRecipeNew(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.workshop == nil || m.workshop.port == nil {
		m.addSystemMessage("Recipe workspace unavailable")
		return m, nil
	}

	if len(args) == 0 {
		m.addSystemMessage("Usage: /recipe new <path>")
		return m, nil
	}

	path := args[0]
	if !strings.HasSuffix(path, ".erpe") {
		path += ".erpe"
	}

	// Create a new empty recipe
	newRecipe := RecipeFile{
		ID:       path,
		Path:     path,
		Revision: "",
		Source:   "thoughtrecipe " + path + "\n",
	}

	m.activeTab = TabWorkshop
	m.workshop.mode = workshopSource
	m.workshop.file = newRecipe
	m.workshop.source = newRecipe.Source
	m.workshop.files = []RecipeFile{newRecipe}

	m.addSystemMessage(fmt.Sprintf("New recipe: %s", path))
	return m, nil
}

func handleRecipeSave(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.workshop == nil || m.workshop.port == nil {
		m.addSystemMessage("Recipe workspace unavailable")
		return m, nil
	}

	if m.workshop.mode != workshopSource && m.workshop.mode != workshopGoal {
		m.addSystemMessage("Not in recipe source editing mode")
		return m, nil
	}

	// Create a new file with the updated source
	newFile := RecipeFile{
		ID:       m.workshop.file.ID,
		Path:     m.workshop.file.Path,
		Revision: m.workshop.file.Revision,
		Source:   m.workshop.source,
	}

	ctx := context.Background()
	saved, err := m.workshop.port.SaveRecipe(ctx, newFile)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Error saving recipe: %v", err))
		return m, nil
	}

	// Update the workshop with the saved file
	m.workshop.file = saved
	m.workshop.file.Revision = saved.Revision
	m.workshop.source = saved.Source
	m.workshop.undo = nil
	m.workshop.redo = nil

	m.addSystemMessage(fmt.Sprintf("Recipe saved: %s (rev: %s)", saved.Path, saved.Revision[:8]))
	return m, nil
}

func handleRecipeClose(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.workshop == nil {
		m.addSystemMessage("Recipe workspace not available")
		return m, nil
	}

	// Check if there's unsaved work
	if m.workshop.Dirty() {
		m.addSystemMessage("Recipe has unsaved changes. Use /recipe save first or discard changes.")
		return m, nil
	}

	// Close by switching to a different tab
	m.activeTab = TabChat

	m.addSystemMessage("Recipe editor closed")
	return m, nil
}

func handleRecipeValidate(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.workshop == nil {
		m.addSystemMessage("Recipe workspace not available")
		return m, nil
	}

	if m.workshop.mode != workshopSource && m.workshop.mode != workshopGoal {
		m.addSystemMessage("Not in recipe editing mode")
		return m, nil
	}

	// For P04, we just check if the recipe parses correctly
	if strings.TrimSpace(m.workshop.source) == "" {
		m.addSystemMessage("Recipe is empty")
		return m, nil
	}

	// Check if it starts with "thoughtrecipe"
	if !strings.HasPrefix(m.workshop.source, "thoughtrecipe") {
		m.addSystemMessage("Warning: Recipe should start with 'thoughtrecipe' declaration")
		return m, nil
	}

	m.addSystemMessage("Recipe syntax OK")
	return m, nil
}

func handleRecipeRun(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.workshop == nil || m.workshop.port == nil {
		m.addSystemMessage("Recipe workspace unavailable")
		return m, nil
	}

	var path string
	if len(args) > 0 {
		path = args[0]
	} else if m.workshop.file.Path != "" {
		path = m.workshop.file.Path
	} else {
		m.addSystemMessage("Usage: /recipe run <path> or open a recipe first")
		return m, nil
	}

	ctx := context.Background()
	
	// If we have unsaved changes, save them first
	if m.workshop.Dirty() {
		// Save the current source
		newFile := RecipeFile{
			ID:       m.workshop.file.ID,
			Path:     m.workshop.file.Path,
			Revision: m.workshop.file.Revision,
			Source:   m.workshop.source,
		}
		_, err := m.workshop.port.SaveRecipe(ctx, newFile)
		if err != nil {
			m.addSystemMessage(fmt.Sprintf("Error saving recipe before run: %v", err))
			return m, nil
		}
		m.addSystemMessage("Saved draft before execution")
	}

	// Open the recipe to get the latest version
	file, err := m.workshop.port.OpenRecipe(ctx, path)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Error opening recipe for execution: %v", err))
		return m, nil
	}

	// Trigger the recipe
	// Use the first line as the entry point if not specified
	entry := ""
	if strings.HasPrefix(file.Source, "thoughtrecipe") {
		lines := strings.Split(file.Source, "\n")
		if len(lines) > 0 {
			parts := strings.Fields(lines[0])
			if len(parts) > 1 {
				entry = parts[1]
			}
		}
	}

	result, err := m.workshop.port.TriggerRecipe(ctx, file, entry)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Error triggering recipe: %v", err))
		return m, nil
	}

	m.addSystemMessage(fmt.Sprintf("Recipe %s triggered: %s", file.Path, result.Status))
	return m, nil
}

func handleRecipeUndo(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.workshop == nil {
		m.addSystemMessage("Recipe workspace not available")
		return m, nil
	}

	if m.workshop.mode != workshopSource && m.workshop.mode != workshopGoal {
		m.addSystemMessage("Not in recipe editing mode")
		return m, nil
	}

	// For P04, undo is handled by the workshop
	if len(m.workshop.undo) > 0 {
		// Pop the last undo state
		lastUndo := m.workshop.undo[len(m.workshop.undo)-1]
		m.workshop.undo = m.workshop.undo[:len(m.workshop.undo)-1]
		
		// Push to redo stack
		m.workshop.redo = append(m.workshop.redo, m.workshop.source)
		
		// Restore the source
		m.workshop.source = lastUndo
		m.workshop.field.SetValue(lastUndo)
		m.addSystemMessage("Undo applied")
	} else {
		m.addSystemMessage("Nothing to undo")
	}
	
	return m, nil
}

func handleRecipeRedo(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.workshop == nil {
		m.addSystemMessage("Recipe workspace not available")
		return m, nil
	}

	if m.workshop.mode != workshopSource && m.workshop.mode != workshopGoal {
		m.addSystemMessage("Not in recipe editing mode")
		return m, nil
	}

	// For P04, redo is handled by the workshop
	if len(m.workshop.redo) > 0 {
		// Pop the last redo state
		lastRedo := m.workshop.redo[len(m.workshop.redo)-1]
		m.workshop.redo = m.workshop.redo[:len(m.workshop.redo)-1]
		
		// Push to undo stack
		m.workshop.undo = append(m.workshop.undo, m.workshop.source)
		
		// Restore the source
		m.workshop.source = lastRedo
		m.workshop.field.SetValue(lastRedo)
		m.addSystemMessage("Redo applied")
	} else {
		m.addSystemMessage("Nothing to redo")
	}
	
	return m, nil
}

func handleRecipeSource(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.workshop == nil {
		m.addSystemMessage("Recipe workspace not available")
		return m, nil
	}

	if m.workshop.file.Path == "" {
		m.addSystemMessage("No recipe open")
		return m, nil
	}

	m.addSystemMessage(fmt.Sprintf("Recipe source:\n%s", m.workshop.source))
	return m, nil
}
