package fmp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type MediationController struct {
	Packager       JSONPackager
	LocalRecipient string
	Now            func() time.Time
}

func QualifiedGatewayRecipient(trustDomain, gatewayID string) string {
	return "gateway://" + strings.TrimSpace(trustDomain) + "/" + strings.TrimSpace(gatewayID)
}

func (m *MediationController) MediateForward(ctx context.Context, svc *Service, req GatewayForwardRequest) (GatewayForwardRequest, *TransferRefusal, error) {
	if !req.MediationRequested {
		return req, nil, nil
	}
	if m == nil {
		return req, &TransferRefusal{Code: RefusalAdmissionClosed, Message: "mediation controller unavailable"}, nil
	}
	if svc == nil {
		return req, nil, fmt.Errorf("fmp service required")
	}
	packager := m.Packager
	if packager.KeyResolver == nil {
		return req, &TransferRefusal{Code: RefusalAdmissionClosed, Message: "mediation packager unavailable"}, nil
	}
	packager.LocalRecipient = strings.TrimSpace(m.LocalRecipient)
	if packager.LocalRecipient == "" {
		return req, &TransferRefusal{Code: RefusalAdmissionClosed, Message: "mediation recipient unavailable"}, nil
	}
	destinationRecipient, err := svc.resolveDestinationRuntimeRecipient(ctx, req)
	if err != nil {
		m.logAudit(ctx, svc, req, "denied", map[string]any{"reason": err.Error()})
		return req, &TransferRefusal{Code: RefusalAdmissionClosed, Message: err.Error()}, nil
	}
	var pkg PortableContextPackage
	if err := packager.UnsealPackage(ctx, req.SealedContext, &pkg); err != nil {
		m.logAudit(ctx, svc, req, "denied", map[string]any{"reason": err.Error()})
		return req, &TransferRefusal{Code: RefusalUnauthorized, Message: "mediation unwrap failed"}, nil
	}
	manifest, err := mediatedManifest(req, req.SealedContext, pkg, nowUTC(m.Now))
	if err != nil {
		m.logAudit(ctx, svc, req, "denied", map[string]any{"reason": err.Error()})
		return req, &TransferRefusal{Code: RefusalUnauthorized, Message: err.Error()}, nil
	}
	pkg.Manifest = manifest
	sealed, err := packager.SealPackage(ctx, manifest, &pkg, []string{destinationRecipient})
	if err != nil {
		m.logAudit(ctx, svc, req, "denied", map[string]any{"reason": err.Error()})
		return req, nil, err
	}
	req.SizeBytes = manifest.SizeBytes
	req.ContextManifestRef = manifest.ContextID
	req.SealedContext = *sealed
	m.logAudit(ctx, svc, req, "ok", map[string]any{
		"destination_recipient": destinationRecipient,
		"context_class":         manifest.ContextClass,
		"sensitivity_class":     string(manifest.SensitivityClass),
		"payload_sha256":        payloadSHA256(pkg.ExecutionPayload),
		"recipient_count":       len(sealed.RecipientBindings),
	})
	return req, nil, nil
}

func (s *Service) resolveDestinationRuntimeRecipient(ctx context.Context, req GatewayForwardRequest) (string, error) {
	if s == nil || s.Discovery == nil {
		return "", fmt.Errorf("discovery store unavailable")
	}
	exports, err := s.listLiveExportAds(ctx)
	if err != nil {
		return "", err
	}
	targetDomain := strings.TrimSpace(req.TrustDomain)
	targetExport := strings.TrimSpace(req.DestinationExport)
	if IsQualifiedExportName(targetExport) {
		if parsedDomain, parsedExport, err := ParseQualifiedExportName(targetExport); err == nil {
			targetDomain = parsedDomain
			targetExport = parsedExport
		}
	}
	for _, ad := range exports {
		if !strings.EqualFold(strings.TrimSpace(ad.TrustDomain), targetDomain) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(ad.Export.ExportName), targetExport) {
			continue
		}
		if strings.TrimSpace(ad.RuntimeID) == "" {
			return "", fmt.Errorf("destination export runtime unresolved")
		}
		return qualifiedRuntimeName(ad.TrustDomain, ad.RuntimeID), nil
	}
	return "", fmt.Errorf("destination export %s not found in trust domain %s", targetExport, targetDomain)
}

func mediatedManifest(req GatewayForwardRequest, sealed SealedContext, pkg PortableContextPackage, now time.Time) (ContextManifest, error) {
	lineageID, ok := stringReplayValue(sealed.ReplayProtectionData, "lineage_id")
	if !ok || strings.TrimSpace(lineageID) == "" {
		lineageID = strings.TrimSpace(req.LineageID)
	}
	attemptID, ok := stringReplayValue(sealed.ReplayProtectionData, "attempt_id")
	if !ok || strings.TrimSpace(attemptID) == "" {
		return ContextManifest{}, fmt.Errorf("sealed context missing attempt_id for mediation")
	}
	contextClass, ok := stringReplayValue(sealed.ReplayProtectionData, "context_class")
	if !ok || strings.TrimSpace(contextClass) == "" {
		return ContextManifest{}, fmt.Errorf("sealed context missing context_class for mediation")
	}
	schemaVersion, ok := stringReplayValue(sealed.ReplayProtectionData, "schema_version")
	if !ok || strings.TrimSpace(schemaVersion) == "" {
		return ContextManifest{}, fmt.Errorf("sealed context missing schema_version for mediation")
	}
	transferMode, _ := stringReplayValue(sealed.ReplayProtectionData, "transfer_mode")
	encryptionMode, _ := stringReplayValue(sealed.ReplayProtectionData, "encryption_mode")
	sensitivityClass, _ := stringReplayValue(sealed.ReplayProtectionData, "sensitivity_class")
	executionHash := sha256.Sum256(pkg.ExecutionPayload)
	manifest := ContextManifest{
		ContextID:        strings.TrimSpace(req.ContextManifestRef),
		LineageID:        strings.TrimSpace(lineageID),
		AttemptID:        strings.TrimSpace(attemptID),
		ContextClass:     strings.TrimSpace(contextClass),
		SchemaVersion:    strings.TrimSpace(schemaVersion),
		SizeBytes:        int64(len(pkg.ExecutionPayload)),
		ChunkCount:       len(sealed.CiphertextChunks) + len(sealed.ExternalObjectRefs),
		ContentHash:      hex.EncodeToString(executionHash[:]),
		SensitivityClass: SensitivityClass(strings.TrimSpace(sensitivityClass)),
		TransferMode:     TransferMode(strings.TrimSpace(transferMode)),
		EncryptionMode:   EncryptionMode(strings.TrimSpace(encryptionMode)),
		RecipientSet:     append([]string(nil), sealed.RecipientBindings...),
		CreationTime:     now.UTC(),
	}
	if manifest.ContextID == "" {
		manifest.ContextID = sealed.ContextManifestRef
	}
	if err := manifest.Validate(); err != nil {
		return ContextManifest{}, err
	}
	return manifest, nil
}

func stringReplayValue(data map[string]any, key string) (string, bool) {
	if data == nil {
		return "", false
	}
	value, ok := data[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return strings.TrimSpace(text), ok
}

func (m *MediationController) logAudit(ctx context.Context, svc *Service, req GatewayForwardRequest, result string, extra map[string]any) {
	if svc == nil || svc.Audit == nil {
		return
	}
	metadata := map[string]any{
		"trust_domain":       req.TrustDomain,
		"source_domain":      req.SourceDomain,
		"destination_export": req.DestinationExport,
		"lineage_id":         req.LineageID,
		"context_manifest":   req.ContextManifestRef,
		"route_mode":         req.RouteMode,
	}
	for k, v := range extra {
		metadata[k] = v
	}
	_ = svc.Audit.Log(ctx, core.AuditRecord{
		Timestamp:  nowUTC(m.Now),
		Action:     "fmp",
		Type:       "fmp.federation.mediated",
		Permission: "mesh",
		Result:     result,
		Metadata:   metadata,
	})
}

func nowUTC(now func() time.Time) time.Time {
	if now != nil {
		return now().UTC()
	}
	return time.Now().UTC()
}

func payloadSHA256(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
