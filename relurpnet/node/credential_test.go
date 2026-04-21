package node

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateCredentialAndVerifyChallenge(t *testing.T) {
	cred, priv, err := GenerateCredential("node-1")
	require.NoError(t, err)

	challenge := []byte("challenge")
	sig := ed25519.Sign(priv, challenge)

	require.NoError(t, VerifyChallenge(cred, challenge, sig))
	require.Error(t, VerifyChallenge(cred, []byte("other"), sig))
}
