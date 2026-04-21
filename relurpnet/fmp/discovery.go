package fmp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// DiscoveryStore is part of the Phase 1 frozen FMP surface.
// The interface is stable even though the current implementation is still an
// eventually consistent in-memory cache.
type DiscoveryStore interface {
	UpsertNodeAdvertisement(ctx context.Context, ad core.NodeAdvertisement) error
	UpsertRuntimeAdvertisement(ctx context.Context, ad core.RuntimeAdvertisement) error
	UpsertExportAdvertisement(ctx context.Context, ad core.ExportAdvertisement) error
	ListNodeAdvertisements(ctx context.Context) ([]core.NodeAdvertisement, error)
	ListRuntimeAdvertisements(ctx context.Context) ([]core.RuntimeAdvertisement, error)
	ListExportAdvertisements(ctx context.Context) ([]core.ExportAdvertisement, error)
	DeleteExpired(ctx context.Context, now time.Time) error
}

type RouteSelectionRequest struct {
	LineageID                  string
	TenantID                   string
	ExportName                 string
	Owner                      core.SubjectRef
	Actor                      core.SubjectRef
	IsOwner                    bool
	IsDelegated                bool
	SessionID                  string
	TrustClass                 core.TrustClass
	TaskClass                  string
	ContextClass               string
	ContextSizeBytes           int64
	SensitivityClass           core.SensitivityClass
	RequiredCompatibilityClass string
	RequiredRouteMode          core.RouteMode
	AllowRemote                bool
}

type RouteCandidate struct {
	QualifiedExport string                  `json:"qualified_export" yaml:"qualified_export"`
	TrustDomain     string                  `json:"trust_domain" yaml:"trust_domain"`
	NodeID          string                  `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	RuntimeID       string                  `json:"runtime_id,omitempty" yaml:"runtime_id,omitempty"`
	Imported        bool                    `json:"imported,omitempty" yaml:"imported,omitempty"`
	RouteMode       core.RouteMode          `json:"route_mode,omitempty" yaml:"route_mode,omitempty"`
	Export          core.ExportDescriptor   `json:"export" yaml:"export"`
	Runtime         *core.RuntimeDescriptor `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Reason          string                  `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type InMemoryDiscoveryStore struct {
	mu       sync.RWMutex
	nodes    map[string]core.NodeAdvertisement
	runtimes map[string]core.RuntimeAdvertisement
	exports  map[string]core.ExportAdvertisement
}

func (s *InMemoryDiscoveryStore) UpsertNodeAdvertisement(_ context.Context, ad core.NodeAdvertisement) error {
	if err := ad.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nodes == nil {
		s.nodes = map[string]core.NodeAdvertisement{}
	}
	s.nodes[qualifiedNodeName(ad.TrustDomain, ad.Node.ID)] = ad
	return nil
}

func (s *InMemoryDiscoveryStore) UpsertRuntimeAdvertisement(_ context.Context, ad core.RuntimeAdvertisement) error {
	if err := ad.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtimes == nil {
		s.runtimes = map[string]core.RuntimeAdvertisement{}
	}
	s.runtimes[qualifiedRuntimeName(ad.TrustDomain, ad.Runtime.RuntimeID)] = ad
	return nil
}

func (s *InMemoryDiscoveryStore) UpsertExportAdvertisement(_ context.Context, ad core.ExportAdvertisement) error {
	if err := ad.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exports == nil {
		s.exports = map[string]core.ExportAdvertisement{}
	}
	s.exports[QualifiedExportName(ad.TrustDomain, ad.Export.ExportName)] = ad
	return nil
}

func (s *InMemoryDiscoveryStore) ListNodeAdvertisements(_ context.Context) ([]core.NodeAdvertisement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.NodeAdvertisement, 0, len(s.nodes))
	for _, ad := range s.nodes {
		out = append(out, ad)
	}
	sort.Slice(out, func(i, j int) bool {
		return qualifiedNodeName(out[i].TrustDomain, out[i].Node.ID) < qualifiedNodeName(out[j].TrustDomain, out[j].Node.ID)
	})
	return out, nil
}

func (s *InMemoryDiscoveryStore) ListRuntimeAdvertisements(_ context.Context) ([]core.RuntimeAdvertisement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.RuntimeAdvertisement, 0, len(s.runtimes))
	for _, ad := range s.runtimes {
		out = append(out, ad)
	}
	sort.Slice(out, func(i, j int) bool {
		return qualifiedRuntimeName(out[i].TrustDomain, out[i].Runtime.RuntimeID) < qualifiedRuntimeName(out[j].TrustDomain, out[j].Runtime.RuntimeID)
	})
	return out, nil
}

func (s *InMemoryDiscoveryStore) ListExportAdvertisements(_ context.Context) ([]core.ExportAdvertisement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.ExportAdvertisement, 0, len(s.exports))
	for _, ad := range s.exports {
		out = append(out, ad)
	}
	sort.Slice(out, func(i, j int) bool {
		return QualifiedExportName(out[i].TrustDomain, out[i].Export.ExportName) < QualifiedExportName(out[j].TrustDomain, out[j].Export.ExportName)
	})
	return out, nil
}

func (s *InMemoryDiscoveryStore) DeleteExpired(_ context.Context, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, ad := range s.nodes {
		if !ad.ExpiresAt.IsZero() && now.After(ad.ExpiresAt) {
			delete(s.nodes, key)
		}
	}
	for key, ad := range s.runtimes {
		if !ad.ExpiresAt.IsZero() && now.After(ad.ExpiresAt) {
			delete(s.runtimes, key)
		}
	}
	for key, ad := range s.exports {
		if !ad.ExpiresAt.IsZero() && now.After(ad.ExpiresAt) {
			delete(s.exports, key)
		}
	}
	return nil
}

func QualifiedExportName(trustDomain, exportName string) string {
	return "mesh://" + strings.TrimSpace(trustDomain) + "/" + strings.TrimSpace(exportName)
}

func qualifiedRuntimeName(trustDomain, runtimeID string) string {
	return "runtime://" + strings.TrimSpace(trustDomain) + "/" + strings.TrimSpace(runtimeID)
}

func qualifiedNodeName(trustDomain, nodeID string) string {
	return "node://" + strings.TrimSpace(trustDomain) + "/" + strings.TrimSpace(nodeID)
}

func IsQualifiedExportName(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "mesh://")
}

func ParseQualifiedExportName(value string) (trustDomain, exportName string, err error) {
	trimmed := strings.TrimSpace(value)
	if !IsQualifiedExportName(trimmed) {
		return "", "", fmt.Errorf("export name %q is not qualified", value)
	}
	rest := strings.TrimPrefix(trimmed, "mesh://")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("export name %q invalid", value)
	}
	return parts[0], parts[1], nil
}

// Phase 6.6: Advertisement TTL Enforcement

// LiveDiscoveryStore is an optional extension that filters expired advertisements at query time.
type LiveDiscoveryStore interface {
	ListLiveNodeAdvertisements(ctx context.Context, now time.Time) ([]core.NodeAdvertisement, error)
	ListLiveRuntimeAdvertisements(ctx context.Context, now time.Time) ([]core.RuntimeAdvertisement, error)
	ListLiveExportAdvertisements(ctx context.Context, now time.Time) ([]core.ExportAdvertisement, error)
}

// ListLiveNodeAdvertisements returns only non-expired node advertisements.
func (s *InMemoryDiscoveryStore) ListLiveNodeAdvertisements(ctx context.Context, now time.Time) ([]core.NodeAdvertisement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []core.NodeAdvertisement
	for _, ad := range s.nodes {
		if ad.ExpiresAt.IsZero() || now.Before(ad.ExpiresAt) {
			out = append(out, ad)
		}
	}
	return out, nil
}

// ListLiveRuntimeAdvertisements returns only non-expired runtime advertisements.
func (s *InMemoryDiscoveryStore) ListLiveRuntimeAdvertisements(ctx context.Context, now time.Time) ([]core.RuntimeAdvertisement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []core.RuntimeAdvertisement
	for _, ad := range s.runtimes {
		if ad.ExpiresAt.IsZero() || now.Before(ad.ExpiresAt) {
			out = append(out, ad)
		}
	}
	return out, nil
}

// ListLiveExportAdvertisements returns only non-expired export advertisements.
func (s *InMemoryDiscoveryStore) ListLiveExportAdvertisements(ctx context.Context, now time.Time) ([]core.ExportAdvertisement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []core.ExportAdvertisement
	for _, ad := range s.exports {
		if ad.ExpiresAt.IsZero() || now.Before(ad.ExpiresAt) {
			out = append(out, ad)
		}
	}
	return out, nil
}
