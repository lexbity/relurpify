package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/git"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
	"github.com/spf13/cobra"
)

var preparedRunCommandExecuteFn = executePreparedRun

func newAgentTestPreparedRunCmd() *cobra.Command {
	var descriptorPath string
	var outputRoot string
	var serviceID string
	var backendProvider string
	var backendFamily string
	var backendEndpoint string
	var backendBinary string
	var backendService string

	cmd := &cobra.Command{
		Use:   "prepared-run",
		Short: "Attach to and inspect a prepared agenttest run descriptor",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(descriptorPath) == "" {
				return fmt.Errorf("--descriptor is required")
			}
			return preparedRunCommandExecuteFn(cmd.Context(), descriptorPath, outputRoot, preparedRunOverrides{
				BackendProvider: backendProvider,
				BackendFamily:   backendFamily,
				BackendEndpoint: backendEndpoint,
				BackendBinary:   backendBinary,
				BackendService:  backendService,
			}, serviceID, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&descriptorPath, "descriptor", "", "Path to a prepared_run.json descriptor")
	cmd.Flags().StringVar(&outputRoot, "output-root", "", "Output root for execution workspace and logs")
	cmd.Flags().StringVar(&serviceID, "restart-service", "", "Restart a service after the workspace opens")
	cmd.Flags().StringVar(&backendProvider, "backend-provider", "", "Override the descriptor backend provider")
	cmd.Flags().StringVar(&backendFamily, "backend-family", "", "Override the descriptor backend family")
	cmd.Flags().StringVar(&backendEndpoint, "backend-endpoint", "", "Override the descriptor backend endpoint")
	cmd.Flags().StringVar(&backendBinary, "backend-bin", "", "Override the descriptor backend binary")
	cmd.Flags().StringVar(&backendService, "backend-service", "", "Override the descriptor backend service")
	return cmd
}

func executePreparedRun(ctx context.Context, descriptorPath string, outputRoot string, overrides preparedRunOverrides, serviceID string, out io.Writer) error {
	return executePreparedRunToWriter(ctx, descriptorPath, outputRoot, overrides, serviceID, out)
}

func executePreparedRunToWriter(ctx context.Context, descriptorPath string, outputRoot string, overrides preparedRunOverrides, serviceID string, out io.Writer) error {
	desc, err := loadPreparedRunDescriptor(descriptorPath)
	if err != nil {
		return err
	}
	target, err := buildPreparedRunWorkspaceTarget(desc, outputRoot, overrides)
	if err != nil {
		return err
	}
	setupLogger, err := newPreparedRunLogger(filepath.Join(target.Descriptor.SetupLogsDir, "agenttest.log"), "[agenttest-setup] ")
	if err != nil {
		return err
	}
	defer func() { _ = setupLogger.Close() }()
	setupTelemetry, err := newPreparedRunTelemetry(filepath.Join(target.Descriptor.SetupTelemetryDir, "agenttest.jsonl"))
	if err != nil {
		return err
	}
	defer func() { _ = setupTelemetry.Close() }()

	executionLogger, err := newPreparedRunLogger(target.Config.LogPath, "[agenttest-run] ")
	if err != nil {
		return err
	}
	defer func() { _ = executionLogger.Close() }()
	executionTelemetry, err := newPreparedRunTelemetry(target.Config.TelemetryPath)
	if err != nil {
		return err
	}
	defer func() { _ = executionTelemetry.Close() }()
	resetContract, err := agenttest.BuildServiceResetContract(target.Descriptor)
	if err != nil {
		return err
	}
	previousGitAvailability := git.SkipAvailabilityProbe
	git.SkipAvailabilityProbe = true
	defer func() { git.SkipAvailabilityProbe = previousGitAvailability }()

	setupLogger.Printf("opening workspace run_id=%s workspace=%s", target.Descriptor.RunID, target.Config.Workspace)
	emitPreparedRunTelemetryEvent(setupTelemetry, "prepared_run.setup_start", preparedRunSetupLogMessage(target), preparedRunTelemetryMetadata(target))
	ws, _, err := openPreparedRunWorkspaceFn(ctx, target.Descriptor, outputRoot, overrides)
	if err != nil {
		setupLogger.Printf("workspace open failed run_id=%s err=%v", target.Descriptor.RunID, err)
		emitPreparedRunTelemetryEvent(setupTelemetry, "prepared_run.setup_error", err.Error(), preparedRunTelemetryMetadata(target))
		return err
	}
	setupLogger.Printf("workspace opened run_id=%s config=%s manifest=%s", target.Descriptor.RunID, target.Config.ConfigPath, target.Config.ManifestPath)
	emitPreparedRunTelemetryEvent(setupTelemetry, "prepared_run.workspace_opened", "workspace opened", preparedRunTelemetryMetadata(target))
	defer func() { _ = ws.Close() }()
	setupLogger.Printf("starting services run_id=%s", target.Descriptor.RunID)
	emitPreparedRunTelemetryEvent(setupTelemetry, "prepared_run.service_start", "starting services", preparedRunTelemetryMetadata(target))
	if err := startPreparedRunServices(ctx, ws); err != nil {
		setupLogger.Printf("service start failed run_id=%s err=%v", target.Descriptor.RunID, err)
		emitPreparedRunTelemetryEvent(setupTelemetry, "prepared_run.service_error", err.Error(), preparedRunTelemetryMetadata(target))
		return err
	}
	if resetContract.RequiresReset() {
		setupLogger.Printf("resetting services run_id=%s strategy=%s", target.Descriptor.RunID, resetContract.Strategy)
		resetMetadata := preparedRunTelemetryMetadata(target)
		resetMetadata["service_reset_strategy"] = resetContract.Strategy
		emitPreparedRunTelemetryEvent(setupTelemetry, "prepared_run.service_reset", "resetting services", resetMetadata)
		if err := resetPreparedRunServices(ctx, ws, resetContract); err != nil {
			setupLogger.Printf("service reset failed run_id=%s err=%v", target.Descriptor.RunID, err)
			emitPreparedRunTelemetryEvent(setupTelemetry, "prepared_run.service_reset_error", err.Error(), resetMetadata)
			return err
		}
	}
	if strings.TrimSpace(serviceID) != "" {
		setupLogger.Printf("restarting service run_id=%s service=%s", target.Descriptor.RunID, serviceID)
		restartMetadata := preparedRunTelemetryMetadata(target)
		restartMetadata["service_id"] = strings.TrimSpace(serviceID)
		emitPreparedRunTelemetryEvent(setupTelemetry, "prepared_run.service_restart", "restarting service", restartMetadata)
		if err := restartPreparedRunService(ctx, ws, serviceID); err != nil {
			setupLogger.Printf("service restart failed run_id=%s service=%s err=%v", target.Descriptor.RunID, serviceID, err)
			emitPreparedRunTelemetryEvent(setupTelemetry, "prepared_run.service_restart_error", err.Error(), restartMetadata)
			return err
		}
		setupLogger.Printf("service restarted run_id=%s service=%s", target.Descriptor.RunID, serviceID)
	}

	services := ws.ListServices()
	sort.Strings(services)
	executionLogger.Printf("execution starting run_id=%s workspace=%s", target.Descriptor.RunID, target.Config.Workspace)
	executionMetadata := preparedRunTelemetryMetadata(target)
	executionMetadata["services"] = services
	executionMetadata["service_id"] = strings.TrimSpace(serviceID)
	emitPreparedRunTelemetryEvent(executionTelemetry, "prepared_run.execution_start", preparedRunExecutionLogMessage(target), executionMetadata)
	if out != nil {
		_, _ = fmt.Fprintf(out, "descriptor: %s\n", descriptorPath)
		_, _ = fmt.Fprintf(out, "workspace: %s\n", target.Config.Workspace)
		_, _ = fmt.Fprintf(out, "config: %s\n", target.Config.ConfigPath)
		_, _ = fmt.Fprintf(out, "manifest: %s\n", target.Descriptor.ManifestPath)
		_, _ = fmt.Fprintf(out, "provider: %s\n", target.Descriptor.BackendProvider)
		_, _ = fmt.Fprintf(out, "backend: %s/%s\n", target.Descriptor.BackendFamily, target.Descriptor.BackendEndpoint)
		if len(services) > 0 {
			_, _ = fmt.Fprintf(out, "services: %s\n", strings.Join(services, ","))
		}
	}
	executionResult, err := executePreparedRunAgentTaskFn(ctx, ws, target.Descriptor, out)
	if err != nil {
		executionLogger.Printf("execution failed run_id=%s err=%v", target.Descriptor.RunID, err)
		executionMetadata["error"] = err.Error()
		emitPreparedRunTelemetryEvent(executionTelemetry, "prepared_run.execution_error", err.Error(), executionMetadata)
		return err
	}
	executionLogger.Printf("execution completed run_id=%s node=%s services=%s", target.Descriptor.RunID, executionResult.NodeID, strings.Join(services, ","))
	executionMetadata["node_id"] = executionResult.NodeID
	emitPreparedRunTelemetryEvent(executionTelemetry, "prepared_run.execution_finish", "prepared run execution complete", executionMetadata)
	report := reportFromPreparedRun(target.Descriptor, target.Config.Workspace, services)
	reportPath := filepath.Join(target.Descriptor.ExecutionDir, "report.json")
	if err := writePreparedRunReport(reportPath, report); err != nil {
		return err
	}
	return nil
}
