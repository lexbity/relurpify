package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory/db"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
	fwnode "github.com/lexcodex/relurpify/framework/middleware/node"
	"github.com/stretchr/testify/require"
)

type fakeNodeChallengeConn struct {
	writes []any
	read   []byte
}

func (c *fakeNodeChallengeConn) WriteJSON(v any) error {
	c.writes = append(c.writes, v)
	return nil
}

func (c *fakeNodeChallengeConn) ReadMessage() (int, []byte, error) {
	return 1, c.read, nil
}

func TestVerifyGatewayNodeChallenge(t *testing.T) {
	store, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer store.Close()

	cred, priv, err := fwnode.GenerateCredential("node-1")
	require.NoError(t, err)
	cred.TenantID = "tenant-1"
	cred.KeyID = "key-1"
	cred.IssuedAt = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.UpsertNodeEnrollment(context.Background(), core.NodeEnrollment{
		TenantID:   "tenant-1",
		NodeID:     "node-1",
		TrustClass: core.TrustClassRemoteApproved,
		Owner: core.SubjectRef{
			TenantID: "tenant-1",
			Kind:     core.SubjectKindNode,
			ID:       "node-1",
		},
		PublicKey:  cred.PublicKey,
		KeyID:      "key-1",
		PairedAt:   cred.IssuedAt,
		AuthMethod: core.AuthMethodNodeChallenge,
	}))

	originalGenerate := generateNodeChallenge
	defer func() { generateNodeChallenge = originalGenerate }()
	nonce := []byte("01234567890123456789012345678901")
	generateNodeChallenge = func() ([]byte, error) { return nonce, nil }

	signature := ed25519.Sign(priv, nonce)
	response, err := json.Marshal(map[string]any{
		"type":      "node.challenge.response",
		"signature": base64.RawURLEncoding.EncodeToString(signature),
	})
	require.NoError(t, err)
	conn := &fakeNodeChallengeConn{read: response}

	err = verifyGatewayNodeChallenge(context.Background(), store, fwgateway.ConnectionPrincipal{
		Authenticated: true,
		Actor: core.EventActor{
			Kind:        "node",
			ID:          "node-1",
			TenantID:    "tenant-1",
			SubjectKind: core.SubjectKindNode,
		},
	}, fwgateway.NodeConnectInfo{NodeID: "node-1"}, conn)
	require.NoError(t, err)
	require.Len(t, conn.writes, 1)
	enrollment, err := store.GetNodeEnrollment(context.Background(), "tenant-1", "node-1")
	require.NoError(t, err)
	require.NotNil(t, enrollment)
	require.False(t, enrollment.LastVerifiedAt.IsZero())
}
