package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNodeDescriptorValidate(t *testing.T) {
	desc := NodeDescriptor{
		ID:         "node-1",
		Name:       "Lex Laptop",
		Platform:   NodePlatformLinux,
		TrustClass: TrustClassWorkspaceTrusted,
	}

	require.NoError(t, desc.Validate())
}

func TestNodeDescriptorValidateRejectsInvalidPlatform(t *testing.T) {
	err := (NodeDescriptor{
		ID:         "node-1",
		Name:       "bad",
		Platform:   NodePlatform("amiga"),
		TrustClass: TrustClassWorkspaceTrusted,
	}).Validate()

	require.Error(t, err)
	require.Contains(t, err.Error(), "platform")
}

func TestNodeCredentialValidate(t *testing.T) {
	now := time.Now().UTC()
	cred := NodeCredential{
		DeviceID:  "device-1",
		PublicKey: []byte("public-key"),
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour),
	}

	require.NoError(t, cred.Validate())
}

func TestNodeCredentialValidateRejectsInvalidExpiry(t *testing.T) {
	now := time.Now().UTC()
	err := (NodeCredential{
		DeviceID:  "device-1",
		PublicKey: []byte("public-key"),
		IssuedAt:  now,
		ExpiresAt: now.Add(-time.Minute),
	}).Validate()

	require.Error(t, err)
	require.Contains(t, err.Error(), "expires_at")
}
