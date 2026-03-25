package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
)

func FederationInboundHandler(mesh *fwfmp.Service, transport *fwgateway.FMPTransportPolicy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mesh == nil || mesh.Forwarder == nil {
			writeFederationForwardResponse(w, http.StatusServiceUnavailable, fwfmp.GatewayForwardTransportResponse{
				Refusal: &core.TransferRefusal{Code: core.RefusalAdmissionClosed, Message: "federation forwarder unavailable"},
			})
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req core.GatewayForwardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeFederationForwardResponse(w, http.StatusBadRequest, fwfmp.GatewayForwardTransportResponse{
				Refusal: &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: err.Error()},
			})
			return
		}
		if err := req.Validate(); err != nil {
			writeFederationForwardResponse(w, http.StatusBadRequest, fwfmp.GatewayForwardTransportResponse{
				Refusal: &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: err.Error()},
			})
			return
		}
		if err := validateFederationTransport(r, transport, req.TrustDomain); err != nil {
			auditFederationTransport(r, mesh, req, "denied", err.Error())
			writeFederationForwardResponse(w, http.StatusForbidden, fwfmp.GatewayForwardTransportResponse{
				Refusal: &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: err.Error()},
			})
			return
		}
		auditFederationTransport(r, mesh, req, "ok", "")
		result, refusal, err := mesh.ForwardFederatedContext(r.Context(), req)
		switch {
		case err != nil:
			writeFederationForwardResponse(w, http.StatusInternalServerError, fwfmp.GatewayForwardTransportResponse{
				Refusal: &core.TransferRefusal{Code: core.RefusalAdmissionClosed, Message: err.Error()},
			})
		case refusal != nil:
			writeFederationForwardResponse(w, http.StatusForbidden, fwfmp.GatewayForwardTransportResponse{Refusal: refusal})
		default:
			writeFederationForwardResponse(w, http.StatusOK, fwfmp.GatewayForwardTransportResponse{Result: result})
		}
	})
}

func validateFederationTransport(r *http.Request, transport *fwgateway.FMPTransportPolicy, trustDomain string) error {
	if transport == nil {
		return nil
	}
	issuedAt, err := parseFederationTransportTime(r.Header.Get(fwgateway.HeaderFMPSessionIssuedAt))
	if err != nil {
		return err
	}
	expiresAt, err := parseFederationTransportTime(r.Header.Get(fwgateway.HeaderFMPSessionExpiresAt))
	if err != nil {
		return err
	}
	return transport.ValidateFederationForward(r.Context(), fwgateway.FederationTransportFrame{
		TrustDomain:      strings.TrimSpace(trustDomain),
		TransportProfile: strings.TrimSpace(r.Header.Get(fwgateway.HeaderFMPTransportProfile)),
		SessionNonce:     strings.TrimSpace(r.Header.Get(fwgateway.HeaderFMPSessionNonce)),
		SessionIssuedAt:  issuedAt,
		SessionExpiresAt: expiresAt,
		PeerKeyID:        strings.TrimSpace(r.Header.Get(fwgateway.HeaderFMPPeerKeyID)),
	}, r.TLS != nil)
}

func parseFederationTransportTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func auditFederationTransport(r *http.Request, mesh *fwfmp.Service, req core.GatewayForwardRequest, result, reason string) {
	if mesh == nil || mesh.Audit == nil {
		return
	}
	metadata := map[string]interface{}{
		"trust_domain":       req.TrustDomain,
		"source_domain":      req.SourceDomain,
		"destination_export": req.DestinationExport,
		"transport_profile":  strings.TrimSpace(r.Header.Get(fwgateway.HeaderFMPTransportProfile)),
		"peer_key_id":        strings.TrimSpace(r.Header.Get(fwgateway.HeaderFMPPeerKeyID)),
		"session_nonce":      strings.TrimSpace(r.Header.Get(fwgateway.HeaderFMPSessionNonce)),
		"session_issued_at":  strings.TrimSpace(r.Header.Get(fwgateway.HeaderFMPSessionIssuedAt)),
		"session_expires_at": strings.TrimSpace(r.Header.Get(fwgateway.HeaderFMPSessionExpiresAt)),
	}
	if reason != "" {
		metadata["reason"] = reason
	}
	_ = mesh.Audit.Log(r.Context(), core.AuditRecord{
		Action:     "fmp",
		Type:       "fmp.federation.forward.transport",
		Permission: "mesh",
		Result:     result,
		Metadata:   metadata,
	})
}

func writeFederationForwardResponse(w http.ResponseWriter, status int, payload fwfmp.GatewayForwardTransportResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
