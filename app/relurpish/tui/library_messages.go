package tui

// LibraryRunRequestedMsg asks the host to stage a recipe run prompt.
type LibraryRunRequestedMsg struct {
	RecipeID string
	Prompt   string
}
