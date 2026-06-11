package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/provider"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

const backgroundDelegationProviderID = "delegation-runtime"

type backgroundDelegationProvider struct {
	mu       sync.Mutex
	runtime  *Runtime
	sessions map[string]*backgroundDelegationSession
}

type backgroundDelegationSession struct {
	snapshot provider.ProviderSessionSnapshot
	cancel   context.CancelFunc
	results  chan fauthorization.BackgroundDelegationOutcome
}

func newBackgroundDelegationProvider() *backgroundDelegationProvider {
	return &backgroundDelegationProvider{
		sessions: map[string]*backgroundDelegationSession{},
	}
}

func (p *backgroundDelegationProvider) Descriptor() provider.ProviderDescriptor {
	return provider.ProviderDescriptor{
		ID:                 backgroundDelegationProviderID,
		Kind:               agentspec.ProviderKindAgentRuntime,
		ActivationScope:    "runtime",
		TrustBaseline:      agentspec.TrustClassBuiltinTrusted,
		RecoverabilityMode: policy.RecoverabilityInProcess,
		SupportsHealth:     true,
		Security: provider.ProviderSecurityProfile{
			Origin:                     agentspec.ProviderOriginLocal,
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

func (p *backgroundDelegationProvider) HealthSnapshot(context.Context) (provider.ProviderHealthSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return provider.ProviderHealthSnapshot{
		Status: "ok",
		Metadata: map[string]any{
			"active_sessions": len(p.sessions),
		},
	}, nil
}

func (p *backgroundDelegationProvider) ListSessions(context.Context) ([]provider.ProviderSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]provider.ProviderSession, 0, len(p.sessions))
	for _, session := range p.sessions {
		out = append(out, session.snapshot.Session)
	}
	return out, nil
}

func (p *backgroundDelegationProvider) SnapshotSessions(context.Context) ([]provider.ProviderSessionSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]provider.ProviderSessionSnapshot, 0, len(p.sessions))
	for _, session := range p.sessions {
		out = append(out, cloneProviderSessionSnapshot(session.snapshot))
	}
	return out, nil
}

func (p *backgroundDelegationProvider) StartBackgroundDelegation(ctx context.Context, request policy.DelegationRequest, target governanceports.DescriptorView, args map[string]any, opts fauthorization.DelegationExecutionOptions) (*fauthorization.BackgroundDelegationHandle, error) {
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
	results := make(chan fauthorization.BackgroundDelegationOutcome, 1)
	session := &backgroundDelegationSession{
		snapshot: provider.ProviderSessionSnapshot{
			Session: provider.ProviderSession{
				ID:             sessionID,
				ProviderID:     p.Descriptor().ID,
				WorkflowID:     request.WorkflowID,
				TaskID:         request.TaskID,
				TrustClass:     agentspec.TrustClass(target.TrustClass()),
				Recoverability: p.Descriptor().RecoverabilityMode,
				CreatedAt:      now,
				LastActivityAt: now,
				Health:         "running",
				Metadata: map[string]any{
					"delegation_id":      request.ID,
					"target_capability":  target.CapabilityID(),
					"target_public_name": target.CapabilityName(),
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
	return &fauthorization.BackgroundDelegationHandle{
		ProviderID:     p.Descriptor().ID,
		SessionID:      sessionID,
		Recoverability: p.Descriptor().RecoverabilityMode,
		Results:        results,
		Cancel: func(ctx context.Context, snapshot policy.DelegationSnapshot) error {
			p.markSession(sessionID, "cancelled", map[string]any{"reason": "delegation cancelled"})
			return p.CloseSession(ctx, sessionID)
		},
	}, nil
}

func (p *backgroundDelegationProvider) runDelegationSession(ctx context.Context, sessionID string, request policy.DelegationRequest, target governanceports.DescriptorView, args map[string]any, opts fauthorization.DelegationExecutionOptions, session *backgroundDelegationSession) {
	defer close(session.results)
	invState := opts.State
	if invState == nil {
		invState = contextdata.NewEnvelopeState(contextdata.NewEnvelope(request.ID, sessionID))
	}
	ps, _ := invState.(ports.State)
	result, err := p.runtime.Tools.InvokeCapability(ctx, ps, target.CapabilityID(), args)
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
	session.results <- fauthorization.BackgroundDelegationOutcome{Result: result, Error: err}
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

func cloneProviderSessionSnapshot(snapshot provider.ProviderSessionSnapshot) provider.ProviderSessionSnapshot {
	out := snapshot
	if snapshot.Session.CapabilityIDs != nil {
		out.Session.CapabilityIDs = append([]string(nil), snapshot.Session.CapabilityIDs...)
	}
	if snapshot.Session.Metadata != nil {
		out.Session.Metadata = map[string]any{}
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
var _ provider.ProviderSessionSnapshotter = (*backgroundDelegationProvider)(nil)
var _ fauthorization.DelegationBackgroundRunner = (*backgroundDelegationProvider)(nil)
