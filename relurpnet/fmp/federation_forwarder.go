package fmp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

)

type FederatedExportHandler func(context.Context, GatewayForwardRequest) (*GatewayForwardResult, error)

type LocalGatewayForwarder struct {
	mu       sync.RWMutex
	handlers map[string]FederatedExportHandler
	now      func() time.Time
}

func NewLocalGatewayForwarder() *LocalGatewayForwarder {
	return &LocalGatewayForwarder{
		now: time.Now,
	}
}

func (f *LocalGatewayForwarder) RegisterExportHandler(trustDomain, exportName string, handler FederatedExportHandler) error {
	if strings.TrimSpace(exportName) == "" {
		return fmt.Errorf("export name required")
	}
	if handler == nil {
		return fmt.Errorf("federated export handler required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.handlers == nil {
		f.handlers = map[string]FederatedExportHandler{}
	}
	f.handlers[f.handlerKey(trustDomain, exportName)] = handler
	return nil
}

func (f *LocalGatewayForwarder) ForwardSealedContext(ctx context.Context, req GatewayForwardRequest) (*GatewayForwardResult, error) {
	handler, ok := f.resolveHandler(req)
	if ok {
		return handler(ctx, req)
	}
	now := time.Now().UTC()
	if f != nil && f.now != nil {
		now = f.now().UTC()
	}
	return &GatewayForwardResult{
		TrustDomain:       req.TrustDomain,
		DestinationExport: req.DestinationExport,
		RouteMode:         req.RouteMode,
		Opaque:            !req.MediationRequested,
		ForwardedAt:       now,
	}, nil
}

func (f *LocalGatewayForwarder) resolveHandler(req GatewayForwardRequest) (FederatedExportHandler, bool) {
	if f == nil {
		return nil, false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.handlers) == 0 {
		return nil, false
	}
	key := f.handlerKey(req.TrustDomain, req.DestinationExport)
	if handler, ok := f.handlers[key]; ok {
		return handler, true
	}
	if IsQualifiedExportName(req.DestinationExport) {
		domain, exportName, err := ParseQualifiedExportName(req.DestinationExport)
		if err == nil {
			if handler, ok := f.handlers[f.handlerKey(domain, exportName)]; ok {
				return handler, true
			}
		}
	}
	if handler, ok := f.handlers[f.handlerKey(req.TrustDomain, unqualifiedExportName(req.DestinationExport))]; ok {
		return handler, true
	}
	return nil, false
}

func (f *LocalGatewayForwarder) handlerKey(trustDomain, exportName string) string {
	return strings.ToLower(strings.TrimSpace(trustDomain)) + "::" + strings.ToLower(strings.TrimSpace(exportName))
}

func unqualifiedExportName(exportName string) string {
	if !IsQualifiedExportName(exportName) {
		return strings.TrimSpace(exportName)
	}
	_, parsed, err := ParseQualifiedExportName(exportName)
	if err != nil {
		return strings.TrimSpace(exportName)
	}
	return parsed
}

var _ GatewayForwarder = (*LocalGatewayForwarder)(nil)
