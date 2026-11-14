package tui

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"time"

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

	// P02 boot reconciliation: runs found nonterminal in the journal after a
	// crash become interrupted here, before the host offers any execution.
	bootNotice := ""
	if rt.Runs != nil {
		if reconciled, err := rt.Runs.ReconcileAbandoned(ctx); err == nil && len(reconciled) > 0 {
			bootNotice = fmt.Sprintf("Reconciled %d interrupted run(s) after restart: %s. Review with /runs; resumption is a new explicit action.",
				len(reconciled), strings.Join(reconciled, ", "))
		}
	}

	recipes := runtimesvc.NewRecipeLibrary(rt.Config.Workspace)

	// P12 retention: bounded sweep of terminal runs past the retention
	// window. Open runs, pinned runs and receipts are never touched.
	if rt.Runs != nil {
		if _, err := rt.Runs.Journal().SweepExpired(ctx, time.Now().UTC().AddDate(0, 0, -30), 100); err != nil {
			// Retention is maintenance, never a boot blocker; the journal
			// keeps serving history and the failure stays observable.
			log.Printf("retention sweep skipped: %v", err)
		}
	}

	// The adapter doubles as the read-only DoctorReader: the startup gate
	// must always receive the boot report, otherwise a fully ready workspace
	// boots locked behind a race with the doctor pane's own refresh.
	var doctorReader DoctorReader
	if reader, ok := adapter.(DoctorReader); ok {
		doctorReader = reader
	}
	m := NewRootModel(ctx, HostOptions{
		Runtime:        adapter,
		SurfaceFactory: factory,
		Recipes:        recipes,
		Workspace:      rt.Config.Workspace,
		BootNotice:     bootNotice,
		UIRecovery:     runtimesvc.NewUIRecoveryStore(rt.Config.Workspace),
		Doctor:         doctorReader,
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
