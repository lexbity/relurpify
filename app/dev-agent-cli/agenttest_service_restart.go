package main

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

func restartPreparedRunService(ctx context.Context, ws *agentenv.Workspace, id string) error {
	if ws == nil {
		return fmt.Errorf("workspace required")
	}
	if id == "" {
		return fmt.Errorf("service id required")
	}
	svc := ws.GetService(id)
	if svc == nil {
		return fmt.Errorf("service %s not found", id)
	}
	if err := svc.Stop(); err != nil {
		return fmt.Errorf("stop service %s: %w", id, err)
	}
	if err := svc.Start(ctx); err != nil {
		return fmt.Errorf("start service %s: %w", id, err)
	}
	return nil
}
