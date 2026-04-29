package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"time"
)

func GenerateCredential(deviceID string) (NodeCredential, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return NodeCredential{}, nil, err
	}
	cred := NodeCredential{
		DeviceID:  deviceID,
		PublicKey: append([]byte(nil), pub...),
		IssuedAt:  time.Now().UTC(),
	}
	return cred, priv, nil
}

func VerifyChallenge(cred NodeCredential, challenge []byte, sig []byte) error {
	if err := cred.Validate(); err != nil {
		return err
	}
	if !cred.ExpiresAt.IsZero() && time.Now().UTC().After(cred.ExpiresAt) {
		return errors.New("credential expired")
	}
	if len(challenge) == 0 {
		return errors.New("challenge required")
	}
	if !ed25519.Verify(ed25519.PublicKey(cred.PublicKey), challenge, sig) {
		return errors.New("signature verification failed")
	}
	return nil
}
