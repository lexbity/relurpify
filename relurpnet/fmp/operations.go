package fmp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type RolloutPolicy struct {
	MinImportRuntimeVersion         string
	AllowedImportRuntimeVersions    []string
	BlockedImportRuntimeVersions    []string
	AllowedImportCompatibilityClass []string
}

type OperationalSnapshot struct {
	WindowStart          time.Time `json:"window_start,omitempty" yaml:"window_start,omitempty"`
	ActiveResumeSlots    int       `json:"active_resume_slots,omitempty" yaml:"active_resume_slots,omitempty"`
	ResumeBytesInWindow  int64     `json:"resume_bytes_in_window,omitempty" yaml:"resume_bytes_in_window,omitempty"`
	ForwardBytesInWindow int64     `json:"forward_bytes_in_window,omitempty" yaml:"forward_bytes_in_window,omitempty"`
	FederatedForwards    int       `json:"federated_forwards,omitempty" yaml:"federated_forwards,omitempty"`
}

type OperationalLimits struct {
	Window                time.Duration `json:"window,omitempty" yaml:"window,omitempty"`
	MaxActiveResumeSlots  int           `json:"max_active_resume_slots,omitempty" yaml:"max_active_resume_slots,omitempty"`
	MaxResumeBytesWindow  int64         `json:"max_resume_bytes_window,omitempty" yaml:"max_resume_bytes_window,omitempty"`
	MaxForwardBytesWindow int64         `json:"max_forward_bytes_window,omitempty" yaml:"max_forward_bytes_window,omitempty"`
	MaxFederatedForwards  int           `json:"max_federated_forwards,omitempty" yaml:"max_federated_forwards,omitempty"`
}

type OperationalLimiter interface {
	AcquireResume(ctx context.Context, slotID string, sizeBytes int64, now time.Time) (*core.TransferRefusal, error)
	ReleaseResume(ctx context.Context, slotID string) error
	AllowForward(ctx context.Context, transferID string, sizeBytes int64, now time.Time) (*core.TransferRefusal, error)
	Snapshot(ctx context.Context, now time.Time) (OperationalSnapshot, error)
}

type InMemoryOperationalLimiter struct {
	Limits OperationalLimits

	mu                 sync.Mutex
	windowStart        time.Time
	activeResumeSlots  map[string]int64
	resumeBytesWindow  int64
	forwardBytesWindow int64
	federatedForwards  int
}

type ManifestDebugView struct {
	ContextID        string                `json:"context_id" yaml:"context_id"`
	LineageID        string                `json:"lineage_id" yaml:"lineage_id"`
	AttemptID        string                `json:"attempt_id" yaml:"attempt_id"`
	ContextClass     string                `json:"context_class" yaml:"context_class"`
	SchemaVersion    string                `json:"schema_version" yaml:"schema_version"`
	SizeBytes        int64                 `json:"size_bytes,omitempty" yaml:"size_bytes,omitempty"`
	ChunkCount       int                   `json:"chunk_count,omitempty" yaml:"chunk_count,omitempty"`
	SensitivityClass core.SensitivityClass `json:"sensitivity_class,omitempty" yaml:"sensitivity_class,omitempty"`
	TransferMode     core.TransferMode     `json:"transfer_mode,omitempty" yaml:"transfer_mode,omitempty"`
	EncryptionMode   core.EncryptionMode   `json:"encryption_mode,omitempty" yaml:"encryption_mode,omitempty"`
	RecipientCount   int                   `json:"recipient_count,omitempty" yaml:"recipient_count,omitempty"`
	ObjectRefCount   int                   `json:"object_ref_count,omitempty" yaml:"object_ref_count,omitempty"`
	CreationTime     time.Time             `json:"creation_time,omitempty" yaml:"creation_time,omitempty"`
}

type SealedContextDebugView struct {
	EnvelopeVersion    string `json:"envelope_version" yaml:"envelope_version"`
	ContextManifestRef string `json:"context_manifest_ref" yaml:"context_manifest_ref"`
	CipherSuite        string `json:"cipher_suite" yaml:"cipher_suite"`
	RecipientCount     int    `json:"recipient_count,omitempty" yaml:"recipient_count,omitempty"`
	ChunkCount         int    `json:"chunk_count,omitempty" yaml:"chunk_count,omitempty"`
	ExternalRefCount   int    `json:"external_ref_count,omitempty" yaml:"external_ref_count,omitempty"`
	HasIntegrityTag    bool   `json:"has_integrity_tag" yaml:"has_integrity_tag"`
}

type TransferDebugView struct {
	TrustDomain string                 `json:"trust_domain,omitempty" yaml:"trust_domain,omitempty"`
	OfferID     string                 `json:"offer_id,omitempty" yaml:"offer_id,omitempty"`
	LeaseID     string                 `json:"lease_id,omitempty" yaml:"lease_id,omitempty"`
	RuntimeID   string                 `json:"runtime_id,omitempty" yaml:"runtime_id,omitempty"`
	RouteMode   core.RouteMode         `json:"route_mode,omitempty" yaml:"route_mode,omitempty"`
	Manifest    ManifestDebugView      `json:"manifest" yaml:"manifest"`
	Sealed      SealedContextDebugView `json:"sealed" yaml:"sealed"`
}

func (l *InMemoryOperationalLimiter) AcquireResume(_ context.Context, slotID string, sizeBytes int64, now time.Time) (*core.TransferRefusal, error) {
	if strings.TrimSpace(slotID) == "" {
		return nil, fmt.Errorf("resume slot id required")
	}
	if sizeBytes < 0 {
		return nil, fmt.Errorf("resume size bytes must be >= 0")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resetWindowLocked(now)
	if l.activeResumeSlots == nil {
		l.activeResumeSlots = map[string]int64{}
	}
	if _, ok := l.activeResumeSlots[slotID]; ok {
		return nil, nil
	}
	if l.Limits.MaxActiveResumeSlots > 0 && len(l.activeResumeSlots) >= l.Limits.MaxActiveResumeSlots {
		return &core.TransferRefusal{Code: core.RefusalDestinationBusy, Message: "resume slot limit reached"}, nil
	}
	if l.Limits.MaxResumeBytesWindow > 0 && l.resumeBytesWindow+sizeBytes > l.Limits.MaxResumeBytesWindow {
		return &core.TransferRefusal{Code: core.RefusalTransferBudget, Message: "resume bandwidth limit reached"}, nil
	}
	l.activeResumeSlots[slotID] = sizeBytes
	l.resumeBytesWindow += sizeBytes
	return nil, nil
}

func (l *InMemoryOperationalLimiter) ReleaseResume(_ context.Context, slotID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.activeResumeSlots == nil {
		return nil
	}
	delete(l.activeResumeSlots, slotID)
	return nil
}

func (l *InMemoryOperationalLimiter) AllowForward(_ context.Context, transferID string, sizeBytes int64, now time.Time) (*core.TransferRefusal, error) {
	if strings.TrimSpace(transferID) == "" {
		return nil, fmt.Errorf("transfer id required")
	}
	if sizeBytes < 0 {
		return nil, fmt.Errorf("forward size bytes must be >= 0")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resetWindowLocked(now)
	if l.Limits.MaxForwardBytesWindow > 0 && l.forwardBytesWindow+sizeBytes > l.Limits.MaxForwardBytesWindow {
		return &core.TransferRefusal{Code: core.RefusalTransferBudget, Message: "forward bandwidth limit reached"}, nil
	}
	if l.Limits.MaxFederatedForwards > 0 && l.federatedForwards >= l.Limits.MaxFederatedForwards {
		return &core.TransferRefusal{Code: core.RefusalDestinationBusy, Message: "federated forward limit reached"}, nil
	}
	l.forwardBytesWindow += sizeBytes
	l.federatedForwards++
	return nil, nil
}

func (l *InMemoryOperationalLimiter) Snapshot(_ context.Context, now time.Time) (OperationalSnapshot, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resetWindowLocked(now)
	return OperationalSnapshot{
		WindowStart:          l.windowStart,
		ActiveResumeSlots:    len(l.activeResumeSlots),
		ResumeBytesInWindow:  l.resumeBytesWindow,
		ForwardBytesInWindow: l.forwardBytesWindow,
		FederatedForwards:    l.federatedForwards,
	}, nil
}

func (l *InMemoryOperationalLimiter) resetWindowLocked(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	window := l.Limits.Window
	if window <= 0 {
		window = time.Minute
	}
	if l.windowStart.IsZero() {
		l.windowStart = now
		return
	}
	if now.Sub(l.windowStart) < window {
		return
	}
	l.windowStart = now
	l.resumeBytesWindow = 0
	l.forwardBytesWindow = 0
	l.federatedForwards = 0
}

func RedactManifest(manifest core.ContextManifest) ManifestDebugView {
	return ManifestDebugView{
		ContextID:        manifest.ContextID,
		LineageID:        manifest.LineageID,
		AttemptID:        manifest.AttemptID,
		ContextClass:     manifest.ContextClass,
		SchemaVersion:    manifest.SchemaVersion,
		SizeBytes:        manifest.SizeBytes,
		ChunkCount:       manifest.ChunkCount,
		SensitivityClass: manifest.SensitivityClass,
		TransferMode:     manifest.TransferMode,
		EncryptionMode:   manifest.EncryptionMode,
		RecipientCount:   len(manifest.RecipientSet),
		ObjectRefCount:   len(manifest.ObjectRefs),
		CreationTime:     manifest.CreationTime,
	}
}

func RedactSealedContext(sealed core.SealedContext) SealedContextDebugView {
	return SealedContextDebugView{
		EnvelopeVersion:    sealed.EnvelopeVersion,
		ContextManifestRef: sealed.ContextManifestRef,
		CipherSuite:        sealed.CipherSuite,
		RecipientCount:     len(sealed.RecipientBindings),
		ChunkCount:         len(sealed.CiphertextChunks),
		ExternalRefCount:   len(sealed.ExternalObjectRefs),
		HasIntegrityTag:    strings.TrimSpace(sealed.IntegrityTag) != "",
	}
}

func BuildTransferDebugView(trustDomain string, offer core.HandoffOffer, runtimeID string, routeMode core.RouteMode, manifest core.ContextManifest, sealed core.SealedContext) TransferDebugView {
	return TransferDebugView{
		TrustDomain: trustDomain,
		OfferID:     offer.OfferID,
		LeaseID:     offer.LeaseToken.LeaseID,
		RuntimeID:   runtimeID,
		RouteMode:   routeMode,
		Manifest:    RedactManifest(manifest),
		Sealed:      RedactSealedContext(sealed),
	}
}

func (s *Service) OperationalSnapshot(ctx context.Context) (OperationalSnapshot, error) {
	if s == nil || s.Limiter == nil {
		return OperationalSnapshot{}, nil
	}
	return s.Limiter.Snapshot(ctx, s.nowUTC())
}

func (s *Service) validateRuntimeImportEligibility(runtime core.RuntimeDescriptor) *core.TransferRefusal {
	policy := s.Rollout
	if len(policy.AllowedImportCompatibilityClass) > 0 && !containsFoldString(policy.AllowedImportCompatibilityClass, runtime.CompatibilityClass) {
		return &core.TransferRefusal{Code: core.RefusalIncompatibleRuntime, Message: "runtime compatibility class blocked by rollout policy"}
	}
	if len(policy.AllowedImportRuntimeVersions) > 0 && !containsFoldString(policy.AllowedImportRuntimeVersions, runtime.RuntimeVersion) {
		return &core.TransferRefusal{Code: core.RefusalIncompatibleRuntime, Message: "runtime version not in rollout allowlist"}
	}
	if len(policy.BlockedImportRuntimeVersions) > 0 && containsFoldString(policy.BlockedImportRuntimeVersions, runtime.RuntimeVersion) {
		return &core.TransferRefusal{Code: core.RefusalIncompatibleRuntime, Message: "runtime version blocked for import"}
	}
	if strings.TrimSpace(policy.MinImportRuntimeVersion) != "" && compareVersion(runtime.RuntimeVersion, policy.MinImportRuntimeVersion) < 0 {
		return &core.TransferRefusal{Code: core.RefusalIncompatibleRuntime, Message: "runtime version below rollout minimum"}
	}
	return nil
}

func (s *Service) reserveResumeSlot(ctx context.Context, slotID string, sizeBytes int64) *core.TransferRefusal {
	if s == nil || s.Limiter == nil {
		return nil
	}
	refusal, err := s.Limiter.AcquireResume(ctx, slotID, sizeBytes, s.nowUTC())
	if err != nil {
		return &core.TransferRefusal{Code: core.RefusalDestinationBusy, Message: err.Error()}
	}
	if refusal != nil {
		s.emit(ctx, core.FrameworkEventFMPRateLimited, core.SubjectRef{}, map[string]any{
			"slot_id":    slotID,
			"size_bytes": sizeBytes,
			"reason":     refusal.Message,
		})
	}
	return refusal
}

func (s *Service) releaseResumeSlot(ctx context.Context, slotID string) {
	if s == nil || s.Limiter == nil || strings.TrimSpace(slotID) == "" {
		return
	}
	_ = s.Limiter.ReleaseResume(ctx, slotID)
}

func (s *Service) allowForwardBudget(ctx context.Context, transferID string, sizeBytes int64) *core.TransferRefusal {
	if s == nil || s.Limiter == nil {
		return nil
	}
	refusal, err := s.Limiter.AllowForward(ctx, transferID, sizeBytes, s.nowUTC())
	if err != nil {
		return &core.TransferRefusal{Code: core.RefusalTransferBudget, Message: err.Error()}
	}
	if refusal != nil {
		s.emit(ctx, core.FrameworkEventFMPRateLimited, core.SubjectRef{}, map[string]any{
			"transfer_id": transferID,
			"size_bytes":  sizeBytes,
			"reason":      refusal.Message,
		})
	}
	return refusal
}

func (s *Service) emitObservability(ctx context.Context, eventType string, owner core.SubjectRef, payload map[string]any) {
	if s == nil {
		return
	}
	if s.Telemetry != nil {
		metadata := map[string]any{}
		for key, value := range payload {
			metadata[key] = value
		}
		metadata["partition"] = s.partition()
		if owner.TenantID != "" {
			metadata["tenant_id"] = owner.TenantID
		}
		s.Telemetry.Emit(core.Event{
			Type:      core.EventStateChange,
			NodeID:    stringValue(payload, "node_id"),
			TaskID:    stringValue(payload, "lineage_id"),
			Message:   eventType,
			Timestamp: s.nowUTC(),
			Metadata:  metadata,
		})
	}
	if s.Audit != nil {
		metadata := map[string]interface{}{}
		for key, value := range payload {
			metadata[key] = value
		}
		if owner.TenantID != "" {
			metadata["tenant_id"] = owner.TenantID
		}
		_ = s.Audit.Log(ctx, core.AuditRecord{
			Timestamp:   s.nowUTC(),
			AgentID:     stringValue(payload, "runtime_id"),
			Action:      "fmp",
			Type:        eventType,
			Permission:  "mesh",
			Result:      "ok",
			Metadata:    metadata,
			Correlation: stringValue(payload, "offer_id"),
		})
	}
}

func compareVersion(left, right string) int {
	leftParts := strings.Split(strings.TrimSpace(left), ".")
	rightParts := strings.Split(strings.TrimSpace(right), ".")
	maxParts := len(leftParts)
	if len(rightParts) > maxParts {
		maxParts = len(rightParts)
	}
	for i := 0; i < maxParts; i++ {
		lv := versionPart(leftParts, i)
		rv := versionPart(rightParts, i)
		if lv < rv {
			return -1
		}
		if lv > rv {
			return 1
		}
	}
	return 0
}

func versionPart(parts []string, index int) int {
	if index >= len(parts) {
		return 0
	}
	value := strings.TrimSpace(parts[index])
	if value == "" {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

func stringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

func (s *Service) partition() string {
	if s == nil || strings.TrimSpace(s.Partition) == "" {
		return "local"
	}
	return s.Partition
}
