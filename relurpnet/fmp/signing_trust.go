package fmp

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"

)

func verifierForTrustBundle(bundle TrustBundle, algorithm string) (PayloadVerifier, bool) {
	if !strings.EqualFold(strings.TrimSpace(algorithm), SignatureAlgorithmEd25519) {
		return nil, false
	}
	for _, anchor := range bundle.TrustAnchors {
		publicKey, ok := parseTrustAnchorPublicKey(anchor, algorithm)
		if !ok {
			continue
		}
		return &Ed25519Verifier{PublicKey: publicKey}, true
	}
	return nil, false
}

func parseTrustAnchorPublicKey(anchor, algorithm string) (ed25519.PublicKey, bool) {
	anchor = strings.TrimSpace(anchor)
	if anchor == "" {
		return nil, false
	}
	prefix := strings.ToLower(strings.TrimSpace(algorithm)) + ":"
	if !strings.HasPrefix(strings.ToLower(anchor), prefix) {
		return nil, false
	}
	encoded := strings.TrimSpace(anchor[len(prefix):])
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, false
	}
	return ed25519.PublicKey(raw), true
}

func trustAnchorForPublicKey(algorithm string, publicKey []byte) string {
	return fmt.Sprintf("%s:%s", strings.ToLower(strings.TrimSpace(algorithm)), base64.RawStdEncoding.EncodeToString(publicKey))
}

func TrustAnchorForSigner(signer PayloadSigner) (string, bool) {
	if signer == nil {
		return "", false
	}
	switch typed := signer.(type) {
	case *Ed25519Signer:
		publicKey := typed.PublicKey()
		if len(publicKey) == 0 {
			return "", false
		}
		return trustAnchorForPublicKey(typed.Algorithm(), publicKey), true
	default:
		return "", false
	}
}
