package main

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

func startPreparedRunServices(ctx context.Context, ws *agentenv.Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace required")
	}
	if ws.ServiceManager == nil {
		return nil
	}
	return ws.ServiceManager.StartAll(ctx)
}

func resetPreparedRunServices(ctx context.Context, ws *agentenv.Workspace, contract agenttest.ServiceResetContract) error {
	if ws == nil {
		return fmt.Errorf("workspace required")
	}
	contract = contract.Normalize()
	switch contract.Strategy {
	case "", "none":
		return nil
	case "restart":
		return ws.Restart(ctx)
	case "stop-start":
		if ws.ServiceManager == nil {
			return nil
		}
		if err := ws.ServiceManager.StopAll(); err != nil {
			return err
		}
		return ws.ServiceManager.StartAll(ctx)
	case "clear":
		if ws.ServiceManager == nil {
			return nil
		}
		if err := ws.ServiceManager.Clear(); err != nil {
			return err
		}
		return ws.ServiceManager.StartAll(ctx)
	default:
		return fmt.Errorf("unsupported service reset strategy %q", contract.Strategy)
	}
}
