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
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/ayenitd"
)

func main() {
	workspace := flag.String("workspace", "", "workspace root the runner operates on")
	stateDir := flag.String("state-dir", "", "runtime state dir (spool, status.json, logs, jobs store)")
	queues := flag.String("queues", "knowledge", "comma-separated claim order")
	workers := flag.Int("workers", 2, "executor worker count")
	refreshInterval := flag.Duration("refresh-interval", 0, "scheduled knowledge.refresh submission cadence (0 = off)")
	flag.Parse()

	if *workspace == "" || *stateDir == "" {
		fmt.Fprintln(os.Stderr, "relurpify-runner: --workspace and --state-dir are required")
		os.Exit(2)
	}

	ctx := context.Background()

	telemetryLog := filepath.Join(*stateDir, "logs", "runner.jsonl")
	if err := os.MkdirAll(filepath.Dir(telemetryLog), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "relurpify-runner: create log dir: %v\n", err)
		os.Exit(1)
	}
	tel, err := newFileTelemetry(telemetryLog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "relurpify-runner: telemetry: %v\n", err)
		os.Exit(1)
	}

	deps, err := ayenitd.BuildKnowledgeRunnerDeps(ctx, *workspace, *stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "relurpify-runner: %v\n", err)
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "relurpify-runner: %v\n", err)
		os.Exit(1)
	}
}
