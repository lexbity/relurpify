package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/identity"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
	fwnode "github.com/lexcodex/relurpify/framework/middleware/node"
)

const defaultTenantID = "local"

type nodeChallengeConn interface {
	WriteJSON(v any) error
	ReadMessage() (messageType int, p []byte, err error)
}

func normalizeTenantID(tenantID string) string {
	if tenantID == "" {
		return defaultTenantID
	}
	return tenantID
}

func verifyGatewayNodeChallenge(ctx context.Context, store identity.Store, principal fwgateway.ConnectionPrincipal, info fwgateway.NodeConnectInfo, conn nodeChallengeConn) error {
	if store == nil {
		return fmt.Errorf("identity store unavailable")
	}
	tenantID := normalizeTenantID(principal.Actor.TenantID)
	nodeID := info.NodeID
	if nodeID == "" {
		nodeID = principal.Actor.ID
	}
	if nodeID == "" {
		return fmt.Errorf("node id required")
	}
	enrollment, err := store.GetNodeEnrollment(ctx, tenantID, nodeID)
	if err != nil {
		return err
	}
	if enrollment == nil {
		return fmt.Errorf("node enrollment not found")
	}
	if principal.Actor.ID != "" && principal.Actor.ID != enrollment.NodeID {
		return fmt.Errorf("principal actor id does not match enrolled node")
	}
	nonce, err := generateNodeChallenge()
	if err != nil {
		return err
	}
	if err := conn.WriteJSON(map[string]any{
		"type":  "node.challenge",
		"nonce": base64.RawURLEncoding.EncodeToString(nonce),
	}); err != nil {
		return err
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	var response struct {
		Type      string `json:"type"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}
	if response.Type != "node.challenge.response" {
		return fmt.Errorf("expected node challenge response")
	}
	signature, err := base64.RawURLEncoding.DecodeString(response.Signature)
	if err != nil {
		return err
	}
	cred := core.NodeCredential{
		DeviceID:  enrollment.NodeID,
		TenantID:  enrollment.TenantID,
		PublicKey: enrollment.PublicKey,
		IssuedAt:  enrollment.PairedAt,
	}
	if err := fwnode.VerifyChallenge(cred, nonce, signature); err != nil {
		return err
	}
	enrollment.LastVerifiedAt = time.Now().UTC()
	if err := store.UpsertNodeEnrollment(ctx, *enrollment); err != nil {
		return err
	}
	return nil
}

var generateNodeChallenge = func() ([]byte, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return nonce, nil
}

func nodeEnrollmentFromPairing(pairing fwnode.PendingPairing) core.NodeEnrollment {
	tenantID := normalizeTenantID(pairing.Cred.TenantID)
	return core.NodeEnrollment{
		TenantID:   tenantID,
		NodeID:     pairing.Cred.DeviceID,
		TrustClass: core.TrustClassRemoteApproved,
		Owner: core.SubjectRef{
			TenantID: tenantID,
			Kind:     core.SubjectKindNode,
			ID:       pairing.Cred.DeviceID,
		},
		PublicKey:  append([]byte(nil), pairing.Cred.PublicKey...),
		KeyID:      pairing.Cred.KeyID,
		PairedAt:   pairing.Cred.IssuedAt,
		AuthMethod: core.AuthMethodNodeChallenge,
	}
}
