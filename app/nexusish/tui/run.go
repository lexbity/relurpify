package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func Run(ctx context.Context, rt RuntimeAdapter) error {
	if rt == nil {
		return fmt.Errorf("runtime is required")
	}
	program := tea.NewProgram(newModel(rt), tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := program.Run()
	return err
}
