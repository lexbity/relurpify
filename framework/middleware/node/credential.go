package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func GenerateCredential(deviceID string) (core.NodeCredential, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return core.NodeCredential{}, nil, err
	}
	cred := core.NodeCredential{
		DeviceID:  deviceID,
		PublicKey: append([]byte(nil), pub...),
		IssuedAt:  time.Now().UTC(),
	}
	return cred, priv, nil
}

func VerifyChallenge(cred core.NodeCredential, challenge []byte, sig []byte) error {
	if err := cred.Validate(); err != nil {
		return err
	}
	if len(challenge) == 0 {
		return errors.New("challenge required")
	}
	if !ed25519.Verify(ed25519.PublicKey(cred.PublicKey), challenge, sig) {
		return errors.New("signature verification failed")
	}
	return nil
}
