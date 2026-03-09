package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
)

const backgroundDelegationProviderID = "delegation-runtime"

type backgroundDelegationProvider struct {
	mu       sync.Mutex
	runtime  *Runtime
	sessions map[string]*backgroundDelegationSession
}

type backgroundDelegationSession struct {
	snapshot core.ProviderSessionSnapshot
	cancel   context.CancelFunc
	results  chan fruntime.BackgroundDelegationOutcome
}

func newBackgroundDelegationProvider() *backgroundDelegationProvider {
	return &backgroundDelegationProvider{
		sessions: map[string]*backgroundDelegationSession{},
	}
}

func (p *backgroundDelegationProvider) Descriptor() core.ProviderDescriptor {
	return core.ProviderDescriptor{
		ID:                 backgroundDelegationProviderID,
		Kind:               core.ProviderKindAgentRuntime,
		ActivationScope:    "runtime",
		TrustBaseline:      core.TrustClassBuiltinTrusted,
		RecoverabilityMode: core.RecoverabilityInProcess,
		SupportsHealth:     true,
		Security: core.ProviderSecurityProfile{
			Origin:                     core.ProviderOriginLocal,
			RequiresFrameworkMediation: true,
		},
	}
}

func (p *backgroundDelegationProvider) Initialize(_ context.Context, rt *Runtime) error {
	if rt == nil || rt.Tools == nil {
		return fmt.Errorf("runtime unavailable")
	}
	p.runtime = rt
	return nil
}

func (p *backgroundDelegationProvider) Close() error {
	p.mu.Lock()
	sessions := make([]*backgroundDelegationSession, 0, len(p.sessions))
	for _, session := range p.sessions {
		sessions = append(sessions, session)
	}
	p.sessions = map[string]*backgroundDelegationSession{}
	p.mu.Unlock()
	for _, session := range sessions {
		session.cancel()
	}
	return nil
}

func (p *backgroundDelegationProvider) CloseSession(_ context.Context, sessionID string) error {
	if p == nil {
		return ErrSessionNotManaged
	}
	p.mu.Lock()
	session, ok := p.sessions[sessionID]
	if ok {
		delete(p.sessions, sessionID)
	}
	p.mu.Unlock()
	if !ok {
		return ErrSessionNotManaged
	}
	session.cancel()
	return nil
}

func (p *backgroundDelegationProvider) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return core.ProviderHealthSnapshot{
		Status: "ok",
		Metadata: map[string]interface{}{
			"active_sessions": len(p.sessions),
		},
	}, nil
}

func (p *backgroundDelegationProvider) ListSessions(context.Context) ([]core.ProviderSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]core.ProviderSession, 0, len(p.sessions))
	for _, session := range p.sessions {
		out = append(out, session.snapshot.Session)
	}
	return out, nil
}

func (p *backgroundDelegationProvider) SnapshotSessions(context.Context) ([]core.ProviderSessionSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]core.ProviderSessionSnapshot, 0, len(p.sessions))
	for _, session := range p.sessions {
		out = append(out, cloneProviderSessionSnapshot(session.snapshot))
	}
	return out, nil
}

func (p *backgroundDelegationProvider) StartBackgroundDelegation(ctx context.Context, request core.DelegationRequest, target core.CapabilityDescriptor, args map[string]any, opts fruntime.DelegationExecutionOptions) (*fruntime.BackgroundDelegationHandle, error) {
	if p == nil || p.runtime == nil || p.runtime.Tools == nil {
		return nil, fmt.Errorf("background delegation provider unavailable")
	}
	sessionCtx, cancel := context.WithCancel(context.Background())
	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				cancel()
			case <-sessionCtx.Done():
			}
		}()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	sessionID := fmt.Sprintf("%s:%s", p.Descriptor().ID, request.ID)
	results := make(chan fruntime.BackgroundDelegationOutcome, 1)
	session := &backgroundDelegationSession{
		snapshot: core.ProviderSessionSnapshot{
			Session: core.ProviderSession{
				ID:             sessionID,
				ProviderID:     p.Descriptor().ID,
				WorkflowID:     request.WorkflowID,
				TaskID:         request.TaskID,
				TrustClass:     target.TrustClass,
				Recoverability: p.Descriptor().RecoverabilityMode,
				CreatedAt:      now,
				LastActivityAt: now,
				Health:         "running",
				Metadata: map[string]interface{}{
					"delegation_id":      request.ID,
					"target_capability":  target.ID,
					"target_public_name": target.Name,
					"task_type":          request.TaskType,
				},
			},
			Metadata: map[string]any{
				"delegation_id": request.ID,
				"background":    true,
			},
			CapturedAt: now,
		},
		cancel:  cancel,
		results: results,
	}
	p.mu.Lock()
	p.sessions[sessionID] = session
	p.mu.Unlock()

	go p.runDelegationSession(sessionCtx, sessionID, request, target, args, opts, session)
	return &fruntime.BackgroundDelegationHandle{
		ProviderID:     p.Descriptor().ID,
		SessionID:      sessionID,
		Recoverability: p.Descriptor().RecoverabilityMode,
		Results:        results,
		Cancel: func(ctx context.Context, snapshot core.DelegationSnapshot) error {
			p.markSession(sessionID, "cancelled", map[string]any{"reason": "delegation cancelled"})
			return p.CloseSession(ctx, sessionID)
		},
	}, nil
}

func (p *backgroundDelegationProvider) runDelegationSession(ctx context.Context, sessionID string, request core.DelegationRequest, target core.CapabilityDescriptor, args map[string]any, opts fruntime.DelegationExecutionOptions, session *backgroundDelegationSession) {
	defer close(session.results)
	state := opts.State
	if state == nil {
		state = core.NewContext()
	}
	result, err := p.runtime.Tools.InvokeCapability(ctx, state, target.ID, args)
	status := "completed"
	if err != nil {
		status = "failed"
	}
	if ctx.Err() != nil {
		status = "cancelled"
	}
	p.markSession(sessionID, status, map[string]any{
		"updated_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
	session.results <- fruntime.BackgroundDelegationOutcome{Result: result, Error: err}
	if status != "running" {
		p.removeSessionLater(sessionID)
	}
	_ = request
}

func (p *backgroundDelegationProvider) markSession(sessionID, health string, metadata map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	session, ok := p.sessions[sessionID]
	if !ok {
		return
	}
	session.snapshot.Session.Health = health
	session.snapshot.Session.LastActivityAt = time.Now().UTC().Format(time.RFC3339Nano)
	for key, value := range metadata {
		if session.snapshot.Metadata == nil {
			session.snapshot.Metadata = map[string]any{}
		}
		session.snapshot.Metadata[key] = value
	}
	session.snapshot.CapturedAt = time.Now().UTC().Format(time.RFC3339Nano)
}

func (p *backgroundDelegationProvider) removeSessionLater(sessionID string) {
	time.AfterFunc(5*time.Second, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.sessions, sessionID)
	})
}

func cloneProviderSessionSnapshot(snapshot core.ProviderSessionSnapshot) core.ProviderSessionSnapshot {
	out := snapshot
	if snapshot.Session.CapabilityIDs != nil {
		out.Session.CapabilityIDs = append([]string(nil), snapshot.Session.CapabilityIDs...)
	}
	if snapshot.Session.Metadata != nil {
		out.Session.Metadata = map[string]interface{}{}
		for key, value := range snapshot.Session.Metadata {
			out.Session.Metadata[key] = value
		}
	}
	if snapshot.Metadata != nil {
		out.Metadata = map[string]any{}
		for key, value := range snapshot.Metadata {
			out.Metadata[key] = value
		}
	}
	return out
}

var _ RuntimeProvider = (*backgroundDelegationProvider)(nil)
var _ DescribedRuntimeProvider = (*backgroundDelegationProvider)(nil)
var _ SessionManagedProvider = (*backgroundDelegationProvider)(nil)
var _ core.ProviderSessionSnapshotter = (*backgroundDelegationProvider)(nil)
var _ fruntime.DelegationBackgroundRunner = (*backgroundDelegationProvider)(nil)
