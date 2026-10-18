// Command relurpify-runner is the ayenitd service runner binary (Q13): a
// spawned, per-workspace process that drains the file spool into the durable
// jobs store and executes knowledge maintenance jobs. It links no named/
// cognitionzoo/app composition; its startup stays lean.
//
// Flags: --workspace W --state-dir S [--queues q1,q2] [--workers N].
// The runner reads configuration through userconfig like everything else and
// reads no process env directly (AGENTS.md).
//
// Exit codes: 0 clean drain; 1 store lock held / runtime failure; 2 usage.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"codeburg.org/lexbit/relurpify/ayenitd"
)

func main() {
	// run returns the exit code instead of calling os.Exit itself so the
	// deferred dependency cleanup (deps.Close, telemetry) always executes;
	// signal delivery is stopped explicitly so os.Exit does not skip it and
	// leave the jobs store lock file unflushed.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stderr)
	stop()
	os.Exit(code)
}

// run parses args, wires the runner, and drains until ctx is cancelled.
// The returned int is the process exit code (0 clean, 1 runtime failure,
// 2 usage).
func run(ctx context.Context, args []string, stderr io.Writer) (exitCode int) {
	flags := flag.NewFlagSet("relurpify-runner", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspace := flags.String("workspace", "", "workspace root the runner operates on")
	stateDir := flags.String("state-dir", "", "runtime state dir (spool, status.json, logs, jobs store)")
	queues := flags.String("queues", "knowledge", "comma-separated claim order")
	workers := flags.Int("workers", 2, "executor worker count")
	refreshInterval := flags.Duration("refresh-interval", 0, "scheduled knowledge.refresh submission cadence (0 = off)")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *workspace == "" || *stateDir == "" {
		_, _ = fmt.Fprintln(stderr, "relurpify-runner: --workspace and --state-dir are required")
		return 2
	}

	telemetryLog := filepath.Join(*stateDir, "logs", "runner.jsonl")
	if err := os.MkdirAll(filepath.Dir(telemetryLog), 0o700); err != nil {
		_, _ = fmt.Fprintf(stderr, "relurpify-runner: create log dir: %v\n", err)
		return 1
	}
	tel, err := newFileTelemetry(telemetryLog)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "relurpify-runner: telemetry: %v\n", err)
		return 1
	}

	deps, err := ayenitd.BuildKnowledgeRunnerDeps(ctx, *workspace, *stateDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "relurpify-runner: %v\n", err)
		return 1
	}
	defer deps.Close()

	cfg := ayenitd.RunnerConfig{
		Workspace:       *workspace,
		StateDir:        *stateDir,
		Queues:          strings.Split(*queues, ","),
		Workers:         *workers,
		RefreshInterval: *refreshInterval,
		Tel:             tel,
		IndexManager:    deps.IndexManager,
		ChunkStore:      deps.ChunkStore,
		Staleness:       deps.Staleness,
	}

	if err := ayenitd.Run(ctx, cfg); err != nil {
		_, _ = fmt.Fprintf(stderr, "relurpify-runner: %v\n", err)
		return 1
	}
	return 0
}
