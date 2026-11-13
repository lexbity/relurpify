package tui

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

type recipeStepSource struct {
	name, strategy, goal, binding, from string
	agent                               *thoughtrecipe.AgentDecl
	run                                 *thoughtrecipe.RunDecl
	goalClause                          *thoughtrecipe.GoalClause
	fromClause                          *thoughtrecipe.FromClause
	captures                            []string
}

func recipeSteps(document *thoughtrecipe.Document) []recipeStepSource {
	if document == nil {
		return nil
	}
	agents := make(map[string]*thoughtrecipe.AgentDecl)
	imports := make(map[string]string)
	for _, decl := range document.Declarations {
		switch item := decl.(type) {
		case *thoughtrecipe.AgentDecl:
			agents[item.Name.Value] = item
		case *thoughtrecipe.ImportDecl:
			if item.Kind == thoughtrecipe.ImportKindPrompt {
				imports[item.Alias.Value] = item.Target.Raw
			}
		}
	}
	var steps []recipeStepSource
	for _, decl := range document.Declarations {
		run, ok := decl.(*thoughtrecipe.RunDecl)
		if !ok {
			continue
		}
		step := recipeStepSource{name: run.Agent.Value, run: run, agent: agents[run.Agent.Value]}
		if step.agent != nil {
			step.strategy = step.agent.AgentType.Value
		}
		for _, clause := range run.Items {
			switch item := clause.(type) {
			case *thoughtrecipe.GoalClause:
				step.goalClause = item
				step.goal = item.Text.Value
				if item.PromptID != nil {
					step.binding = imports[item.PromptID.Name.Value]
					if step.binding == "" {
						step.binding = "unresolved: " + item.PromptID.Name.Value
					}
				}
			case *thoughtrecipe.FromClause:
				step.fromClause = item
				step.from = sourceValue(item.Source)
			case *thoughtrecipe.CaptureBlock:
				for _, binding := range item.Bindings {
					step.captures = append(step.captures, binding.Destination.Raw)
				}
			}
		}
		steps = append(steps, step)
	}
	return steps
}

func sourceValue(value thoughtrecipe.ValueExpr) string {
	switch item := value.(type) {
	case thoughtrecipe.PathExpr:
		return item.Raw
	case *thoughtrecipe.Identifier:
		return item.Value
	case *thoughtrecipe.StringLiteral:
		return item.Raw
	case *thoughtrecipe.ListLiteral:
		return item.Raw
	}
	return ""
}

// Source columns count runes; patches operate on bytes. End spans in the
// existing AST are inclusive and are unreliable for multiline literals, so
// edits use a start coordinate plus the exact original token bytes instead.
func recipeOffset(source string, location thoughtrecipe.SourceLocation) (int, error) {
	line, column := 1, 1
	for offset, r := range source {
		if line == location.Line && column == location.Column {
			return offset, nil
		}
		if r == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	if line == location.Line && column == location.Column {
		return len(source), nil
	}
	return 0, fmt.Errorf("source coordinate %d:%d is outside the document", location.Line, location.Column)
}

func patchRecipeToken(source string, location thoughtrecipe.SourceLocation, original, replacement string) (string, error) {
	start, err := recipeOffset(source, location)
	if err != nil {
		return "", err
	}
	if original == "" || !strings.HasPrefix(source[start:], original) {
		return "", fmt.Errorf("source token changed; reopen the recipe before editing")
	}
	return source[:start] + replacement + source[start+len(original):], nil
}

func patchRecipeStrategy(source string, step recipeStepSource, strategy string) (string, error) {
	if step.agent == nil {
		return "", fmt.Errorf("run has no locally editable agent declaration")
	}
	return patchRecipeToken(source, step.agent.AgentType.GetSpan().Start, step.agent.AgentType.Value, strategy)
}

func patchRecipeGoal(source string, step recipeStepSource, text string) (string, error) {
	if step.goalClause == nil {
		return "", fmt.Errorf("run has no goal clause; use source editing to add it")
	}
	if strings.Contains(text, `"""`) {
		return "", fmt.Errorf("goal contains a triple-quote delimiter; edit the source explicitly")
	}
	original, err := goalClauseSource(source, step.goalClause)
	if err != nil {
		return "", err
	}
	return patchRecipeToken(source, step.goalClause.GetSpan().Start, original, "goal "+`"""`+text+`"""`)
}

func goalClauseSource(source string, goal *thoughtrecipe.GoalClause) (string, error) {
	start, err := recipeOffset(source, goal.GetSpan().Start)
	if err != nil {
		return "", err
	}
	var end int
	if goal.PromptID != nil {
		offset, err := recipeOffset(source, goal.PromptID.Name.GetSpan().Start)
		if err != nil {
			return "", err
		}
		end = offset + len(goal.PromptID.Name.Value)
	} else {
		offset, err := recipeOffset(source, goal.Text.GetSpan().Start)
		if err != nil {
			return "", err
		}
		end = offset + len(goal.Text.Raw)
	}
	if end < start || end > len(source) {
		return "", fmt.Errorf("invalid goal source span")
	}
	return source[start:end], nil
}

func patchRecipeConnection(source string, step recipeStepSource, connection string) (string, error) {
	if step.fromClause == nil || step.from == "" {
		return "", fmt.Errorf("run has no editable from clause; use source editing")
	}
	return patchRecipeToken(source, step.fromClause.Source.GetSpan().Start, step.from, connection)
}

func patchRecipePrompt(source string, document *thoughtrecipe.Document, step recipeStepSource, promptID string) (string, error) {
	if step.goalClause == nil {
		return "", fmt.Errorf("run has no goal clause")
	}
	alias := ""
	used := make(map[string]bool)
	for _, decl := range document.Declarations {
		if imp, ok := decl.(*thoughtrecipe.ImportDecl); ok {
			used[imp.Alias.Value] = true
			if imp.Kind == thoughtrecipe.ImportKindPrompt && imp.Target.Raw == promptID {
				alias = imp.Alias.Value
			}
		}
	}
	newImport := alias == ""
	if newImport {
		for i := 1; ; i++ {
			alias = fmt.Sprintf("bound_prompt_%d", i)
			if !used[alias] {
				break
			}
		}
	}
	original, err := goalClauseSource(source, step.goalClause)
	if err != nil {
		return "", err
	}
	changed, err := patchRecipeToken(source, step.goalClause.GetSpan().Start, original, "goal prompt "+alias)
	if err != nil {
		return "", err
	}
	if !newImport {
		return changed, nil
	}
	// Imports may follow executable declarations in the real grammar. Appending
	// avoids reserializing/repositioning comments and unrelated declarations.
	return strings.TrimRight(changed, "\n") + "\n\nimport prompt " + promptID + " as " + alias + "\n", nil
}

func recipeConnections(document *thoughtrecipe.Document, selected int) []string {
	var values []string
	for _, decl := range document.Declarations {
		if input, ok := decl.(*thoughtrecipe.InputDecl); ok {
			values = append(values, "input."+input.Name.Value)
		}
	}
	steps := recipeSteps(document)
	for _, step := range steps[:min(selected, len(steps))] {
		values = append(values, step.captures...)
	}
	slices.Sort(values)
	return slices.Compact(values)
}

func validRecipeIdentifier(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	for i, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
