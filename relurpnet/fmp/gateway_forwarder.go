package fmp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	fwgateway "codeburg.org/lexbit/relurpify/relurpnet/gateway"
)

const DefaultFederationForwardPath = "/federation/forward"

type FederationEndpointResolver interface {
	ResolveFederationEndpoint(ctx context.Context, trustDomain string) (string, bool)
}

type StaticFederationEndpointResolver map[string]string

func (r StaticFederationEndpointResolver) ResolveFederationEndpoint(_ context.Context, trustDomain string) (string, bool) {
	endpoint, ok := r[strings.ToLower(strings.TrimSpace(trustDomain))]
	return strings.TrimSpace(endpoint), ok && strings.TrimSpace(endpoint) != ""
}

type GatewayForwardTransportResponse struct {
	Result  *GatewayForwardResult `json:"result,omitempty"`
	Refusal *TransferRefusal      `json:"refusal,omitempty"`
}

type HTTPGatewayForwarder struct {
	Client          *http.Client
	Resolver        FederationEndpointResolver
	ForwardPath     string
	Headers         map[string]string
	TransportPolicy *fwgateway.FMPTransportPolicy
	mu              sync.RWMutex
	localHandlers   map[string]FederatedExportHandler
	now             func() time.Time
}

func NewHTTPGatewayForwarder(resolver FederationEndpointResolver) *HTTPGatewayForwarder {
	return &HTTPGatewayForwarder{
		Client:      &http.Client{Timeout: 10 * time.Second},
		Resolver:    resolver,
		ForwardPath: DefaultFederationForwardPath,
		now:         time.Now,
	}
}

func (f *HTTPGatewayForwarder) RegisterExportHandler(trustDomain, exportName string, handler FederatedExportHandler) error {
	if strings.TrimSpace(exportName) == "" {
		return fmt.Errorf("export name required")
	}
	if handler == nil {
		return fmt.Errorf("federated export handler required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.localHandlers == nil {
		f.localHandlers = map[string]FederatedExportHandler{}
	}
	f.localHandlers[f.handlerKey(trustDomain, exportName)] = handler
	return nil
}

func (f *HTTPGatewayForwarder) ForwardSealedContext(ctx context.Context, req GatewayForwardRequest) (*GatewayForwardResult, error) {
	if handler, ok := f.resolveLocalHandler(req); ok {
		return handler(ctx, req)
	}
	endpoint, ok := f.resolveEndpoint(ctx, req.TrustDomain)
	if !ok {
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
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, joinFederationPath(endpoint, f.forwardPath()), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for key, value := range f.Headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			httpReq.Header.Set(key, value)
		}
	}
	f.applyTransportHeaders(httpReq, req, endpoint)
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var envelope GatewayForwardTransportResponse
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return nil, err
		}
	}
	if resp.StatusCode >= 400 {
		if envelope.Refusal != nil {
			return nil, fmt.Errorf("gateway forward refused: %s", envelope.Refusal.Message)
		}
		return nil, fmt.Errorf("gateway forward failed: %s", resp.Status)
	}
	if envelope.Refusal != nil {
		return nil, fmt.Errorf("gateway forward refused: %s", envelope.Refusal.Message)
	}
	if envelope.Result == nil {
		return nil, fmt.Errorf("gateway forward response missing result")
	}
	return envelope.Result, nil
}

func (f *HTTPGatewayForwarder) resolveEndpoint(ctx context.Context, trustDomain string) (string, bool) {
	if f == nil || f.Resolver == nil {
		return "", false
	}
	return f.Resolver.ResolveFederationEndpoint(ctx, trustDomain)
}

func (f *HTTPGatewayForwarder) resolveLocalHandler(req GatewayForwardRequest) (FederatedExportHandler, bool) {
	if f == nil {
		return nil, false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.localHandlers) == 0 {
		return nil, false
	}
	key := f.handlerKey(req.TrustDomain, req.DestinationExport)
	if handler, ok := f.localHandlers[key]; ok {
		return handler, true
	}
	if IsQualifiedExportName(req.DestinationExport) {
		domain, exportName, err := ParseQualifiedExportName(req.DestinationExport)
		if err == nil {
			if handler, ok := f.localHandlers[f.handlerKey(domain, exportName)]; ok {
				return handler, true
			}
		}
	}
	if handler, ok := f.localHandlers[f.handlerKey(req.TrustDomain, unqualifiedExportName(req.DestinationExport))]; ok {
		return handler, true
	}
	return nil, false
}

func (f *HTTPGatewayForwarder) handlerKey(trustDomain, exportName string) string {
	return strings.ToLower(strings.TrimSpace(trustDomain)) + "::" + strings.ToLower(strings.TrimSpace(exportName))
}

func (f *HTTPGatewayForwarder) forwardPath() string {
	if f == nil || strings.TrimSpace(f.ForwardPath) == "" {
		return DefaultFederationForwardPath
	}
	return strings.TrimSpace(f.ForwardPath)
}

func joinFederationPath(base, path string) string {
	return strings.TrimRight(strings.TrimSpace(base), "/") + "/" + strings.TrimLeft(strings.TrimSpace(path), "/")
}

func (f *HTTPGatewayForwarder) applyTransportHeaders(httpReq *http.Request, req GatewayForwardRequest, endpoint string) {
	if f == nil || httpReq == nil || f.TransportPolicy == nil {
		return
	}
	now := time.Now().UTC()
	if f.now != nil {
		now = f.now().UTC()
	}
	ttl := f.TransportPolicy.SessionTTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	profile := fwgateway.TransportProfileHTTPTLS
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(endpoint)), "http://") {
		profile = fwgateway.TransportProfileHTTPLoopback
	}
	httpReq.Header.Set("X-FMP-Trust-Domain", strings.TrimSpace(req.TrustDomain))
	httpReq.Header.Set(fwgateway.HeaderFMPTransportProfile, profile)
	httpReq.Header.Set(fwgateway.HeaderFMPSessionNonce, generateFederationNonce())
	httpReq.Header.Set(fwgateway.HeaderFMPSessionIssuedAt, now.Format(time.RFC3339Nano))
	httpReq.Header.Set(fwgateway.HeaderFMPSessionExpiresAt, now.Add(ttl).Format(time.RFC3339Nano))
	peerKeyID := strings.TrimSpace(req.GatewayIdentity.ID)
	if peerKeyID == "" {
		peerKeyID = "gateway"
	}
	httpReq.Header.Set(fwgateway.HeaderFMPPeerKeyID, peerKeyID)
}

func generateFederationNonce() string {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("nonce-%d", time.Now().UTC().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

var _ GatewayForwarder = (*HTTPGatewayForwarder)(nil)
