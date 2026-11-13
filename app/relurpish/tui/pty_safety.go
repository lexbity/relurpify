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
func RunWithSurface(ctx context.Context, rt *runtimesvc.Runtime, factory SurfaceFactory) error {
	if rt == nil {
		return fmt.Errorf("runtime is required")
	}
	adapter := NewRuntimeAdapter(rt)
	
	// Get recipe workspace from runtime
	// TODO: P04 - Wire RecipeWorkspace properly
	var recipes RecipeWorkspace
	// if recipeSvc, err := rt.RecipeWorkspace(); err == nil {
	// 	recipes = recipeSvc
	// }
	
	m := NewRootModel(ctx, HostOptions{
		Runtime:  adapter,
		SurfaceFactory: factory,
		Recipes: recipes,
		// TODO: P04 - Fix Workspace.Root()
		Workspace: "",
	})
	return runModel(ctx, m)
}

// RunHost composes application ports into the same host used by live execution.
// The constructor enforces separation of explicit demo and live dependencies.
func RunHost(ctx context.Context, options HostOptions) error {
	return PTYSafe(func() error { return runModel(ctx, NewRootModel(ctx, options)) })
}

// RunDiagnostics opens the same host when execution cannot be constructed.
// No execution adapter is synthesized; ordinary requests remain drafts.
func RunDiagnostics(ctx context.Context, workspace string, reader DoctorReader, factory SurfaceFactory) error {
	m := NewRootModel(ctx, HostOptions{SurfaceFactory: factory, Workspace: workspace, Doctor: reader})
	m.doctorReader = reader
	m.sharedSess.Workspace = workspace
	m.startupLocked = true
	m.setActiveTab(TabDoctor)
	return runModel(ctx, m)
}

func runModel(ctx context.Context, m RootModel) error {
	defer m.cleanup()
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
