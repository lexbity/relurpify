// runner.go is the ayenitd Runner: open the durable store (the Badger
// directory lock IS the single-executor invariant — a second opener fails
// here with runner.lock_contented telemetry and a nonzero exit), recover
// interrupted jobs (store-internal, S7), supervise the spool watcher, the
// executor, and the status writer through execution/services, and drain on
// SIGTERM/SIGINT per Q16 (stop claiming → cancel attempts → wait ≤ 20 s →
// flush status.json with draining → exit 0).
package ayenitd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/execution/services"
	"codeburg.org/lexbit/relurpify/jobs"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// RunnerConfig carries the runner's startup parameters (mirrors the main.go
// flags; the runner reads no process env directly).
type RunnerConfig struct {
	Workspace       string        // workspace root the handlers operate on
	StateDir        string        // <state>/jobs/{spool,status.json,logs}
	StoreDir        string        // Badger directory (default <state>/jobs/store)
	Queues          []string      // claim order
	Workers         int           // executor worker pool (default 2)
	RefreshInterval time.Duration // scheduled knowledge.refresh submission (0 = off, FR-23)
	PollEvery       time.Duration // spool poll interval (default 500 ms)
	Heartbeat       time.Duration // status heartbeat (default 5 s)
	Grace           time.Duration // shutdown handler grace (default 20 s)
	RunnerID        string        // instance identity (default hostname-pid)
	Tel             telemetry.Telemetry
	IndexManager    *ast.IndexManager // knowledge.bootstrap handler dependency
	ChunkStore      *knowledge.ChunkStore
	Staleness       *knowledge.StalenessManager
}

// Run opens the store, supervises the three runner-internal services, and
// blocks until ctx is cancelled or SIGTERM/SIGINT arrives. The returned
// error distinguishes exit codes in main: a lock failure is the nonzero
// attach-don't-fight path (FR-17).
func Run(ctx context.Context, cfg RunnerConfig) error {
	if cfg.StateDir == "" {
		return fmt.Errorf("runner: state dir required")
	}
	storeDir := cfg.StateDir + "/jobs/store"
	if cfg.RunnerID == "" {
		host, _ := os.Hostname()
		cfg.RunnerID = fmt.Sprintf("%s-%d", host, os.Getpid())
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 2
	}
	if len(cfg.Queues) == 0 {
		cfg.Queues = []string{"knowledge"}
	}
	if cfg.Grace <= 0 {
		cfg.Grace = 20 * time.Second
	}
	if cfg.Tel != nil {
		cfg.Tel.Emit(telemetry.Event{Type: telemetry.EventRunnerStarted, Message: "runner starting", Timestamp: time.Now().UTC(),
			Metadata: map[string]any{"runner_id": cfg.RunnerID, "workspace": cfg.Workspace}})
	}

	storeIface, err := jobsOpen(cfg)
	if err != nil {
		if cfg.Tel != nil {
			cfg.Tel.Emit(telemetry.Event{Type: telemetry.EventRunnerLockContented, Message: "jobs store lock held by another runner", Timestamp: time.Now().UTC(),
				Metadata: map[string]any{"store_dir": storeDir}})
		}
		return fmt.Errorf("runner: open jobs store (lock held?): %w", err)
	}
	defer func() {
		if closer, ok := storeIface.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()

	spoolDirs, err := OpenSpoolDirs(cfg.StateDir)
	if err != nil {
		return fmt.Errorf("runner: spool: %w", err)
	}

	handlers := handlerRegistry{}
	if cfg.IndexManager != nil {
		handlers["knowledge.bootstrap"] = knowledge.NewBootstrapHandler(cfg.IndexManager, cfg.Tel)
	}
	if cfg.ChunkStore != nil && cfg.Staleness != nil {
		handlers["knowledge.refresh"] = knowledge.NewRefreshHandler(cfg.ChunkStore, cfg.Staleness, cfg.Workspace, storeIface, cfg.Tel)
	}

	mgr := services.NewServiceManager()
	mgr.RegisterWithInfo("spool-watcher", newSpoolWatcher(spoolDirs, storeIface, cfg.Tel),
		services.ServiceRegistrationInfo{Source: "internal", Owner: "ayenitd", Notes: []string{"pending→claimed→store pipeline"}})
	mgr.RegisterWithInfo("executor", newExecutor(storeIface, handlers, cfg.Tel, cfg.Workers, cfg.Queues),
		services.ServiceRegistrationInfo{Source: "internal", Owner: "ayenitd", Notes: []string{fmt.Sprintf("%d workers", cfg.Workers)}})
	statusSrv := newStatusWriter(cfg.StateDir, storeDir, cfg.RunnerID, storeIface, cfg.Heartbeat)
	mgr.RegisterWithInfo("status-writer", statusSrv,
		services.ServiceRegistrationInfo{Source: "internal", Owner: "ayenitd", Notes: []string{"5 s heartbeat"}})

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := mgr.StartAll(runCtx); err != nil {
		return fmt.Errorf("runner: start services: %w", err)
	}

	// Signal handling (Q16): SIGTERM/SIGINT → drain.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	select {
	case <-ctx.Done():
	case sig := <-sigCh:
		fmt.Printf("runner: received %s; draining\n", sig)
	}

	// Drain: stop claiming (cancel executor ctx), wait ≤ grace for in-flight
	// handlers, flush status.json with draining=true, exit 0. StopAll joins
	// the worker goroutines; the grace bounds the wait (NFR-2 SIGTERM→exit
	// ≤ 25 s = 20 s grace + 5 s flush) via a watchdog that hard-cancels.
	graceTimer := time.AfterFunc(cfg.Grace, cancel)
	defer graceTimer.Stop()
	if err := mgr.StopAll(); err != nil {
		fmt.Printf("runner: service stop errors: %v\n", err)
	}
	if cfg.Tel != nil {
		cfg.Tel.Emit(telemetry.Event{Type: telemetry.EventRunnerStopped, Message: "runner stopped", Timestamp: time.Now().UTC(),
			Metadata: map[string]any{"runner_id": cfg.RunnerID}})
	}
	return nil
}

// jobsOpen is the store-open seam (tests inject failures).
var jobsOpen = func(cfg RunnerConfig) (jobs.Store, error) {
	return jobsStoreOpen(cfg.StateDir + "/jobs/store")
}
