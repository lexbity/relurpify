package tui

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
)

// PTYSafe wraps an arbitrary function with the terminal recovery gate so that
// CLI entrypoints (runTUI, runDoctor, etc.) are protected even if a panic
// occurs outside of Run/RunWithSurface.
func PTYSafe(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprint(os.Stderr, "\033[?1049l")
			fmt.Fprint(os.Stderr, "\033[?25h")
			fmt.Fprint(os.Stderr, "\033[0m")
			log.Printf("relurpish entrypoint panic recovered: %v", r)
			debug.PrintStack()
			err = fmt.Errorf("entrypoint panic: %v", r)
		}
	}()
	return fn()
}

// Run bootstraps the TUI with the default interaction surface factory.
func Run(ctx context.Context, rt *runtimesvc.Runtime) error {
	return RunWithSurface(ctx, rt, NewDefaultSurfaceFactory())
}

// RunWithSurface bootstraps the TUI with an agent-surface factory.
// A deferred panic recovery gate ensures the terminal is restored to a sane
// state even if the program panics during rendering or event handling.
func RunWithSurface(ctx context.Context, rt *runtimesvc.Runtime, factory SurfaceFactory) (err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprint(os.Stderr, "\033[?1049l") // exit alternate screen
			fmt.Fprint(os.Stderr, "\033[?25h")   // restore cursor visibility
			fmt.Fprint(os.Stderr, "\033[0m")     // reset color attributes
			log.Printf("relurpish panic recovered: %v", r)
			debug.PrintStack()
			err = fmt.Errorf("relurpish panic: %v", r)
		}
	}()

	if rt == nil {
		return fmt.Errorf("runtime is required")
	}
	adapter := newRuntimeAdapter(rt)
	m := newRootModel(adapter, factory)
	program := tea.NewProgram(
		m,
		tea.WithContext(ctx),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	final, progErr := program.Run()
	if rm, ok := final.(RootModel); ok {
		rm.cleanup()
	}
	if progErr != nil {
		return progErr
	}
	return nil
}
