package fmp

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

const SignatureAlgorithmEd25519 = "ed25519"

type PayloadSigner interface {
	Algorithm() string
	SignPayload(payload []byte) (string, error)
}

type PayloadVerifier interface {
	VerifyPayload(payload []byte, algorithm, signature string) error
}

type Ed25519Signer struct {
	PrivateKey ed25519.PrivateKey
}

func NewEd25519Signer() (*Ed25519Signer, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Ed25519Signer{PrivateKey: privateKey}, nil
}

func NewEd25519SignerFromSeed(seed []byte) *Ed25519Signer {
	sum := sha256.Sum256(seed)
	return &Ed25519Signer{PrivateKey: ed25519.NewKeyFromSeed(sum[:])}
}

func (s *Ed25519Signer) Algorithm() string {
	return SignatureAlgorithmEd25519
}

func (s *Ed25519Signer) SignPayload(payload []byte) (string, error) {
	if s == nil || len(s.PrivateKey) == 0 {
		return "", fmt.Errorf("ed25519 private key required")
	}
	return base64.RawStdEncoding.EncodeToString(ed25519.Sign(s.PrivateKey, payload)), nil
}

func (s *Ed25519Signer) PublicKey() ed25519.PublicKey {
	if s == nil || len(s.PrivateKey) == 0 {
		return nil
	}
	return append(ed25519.PublicKey(nil), s.PrivateKey.Public().(ed25519.PublicKey)...)
}

type Ed25519Verifier struct {
	PublicKey ed25519.PublicKey
}

func (v *Ed25519Verifier) VerifyPayload(payload []byte, algorithm, signature string) error {
	if !strings.EqualFold(strings.TrimSpace(algorithm), SignatureAlgorithmEd25519) {
		return fmt.Errorf("unsupported signature algorithm %q", algorithm)
	}
	if v == nil || len(v.PublicKey) == 0 {
		return fmt.Errorf("ed25519 public key required")
	}
	sig, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return err
	}
	if !ed25519.Verify(v.PublicKey, payload, sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func SignContextManifest(signer PayloadSigner, manifest *core.ContextManifest) error {
	if signer == nil || manifest == nil {
		return nil
	}
	return signPayloadObject(signer, "context_manifest", manifest, func() {
		manifest.SignatureAlgorithm = signer.Algorithm()
	}, func() string { return manifest.Signature }, func(v string) { manifest.Signature = v })
}

func VerifyContextManifest(verifier PayloadVerifier, manifest core.ContextManifest) error {
	return verifyPayloadObject(verifier, "context_manifest", manifest, manifest.SignatureAlgorithm, manifest.Signature)
}

func SignLeaseToken(signer PayloadSigner, token *core.LeaseToken) error {
	if signer == nil || token == nil {
		return nil
	}
	return signPayloadObject(signer, "lease_token", token, func() {
		token.SignatureAlgorithm = signer.Algorithm()
	}, func() string { return token.Signature }, func(v string) { token.Signature = v })
}

func VerifyLeaseToken(verifier PayloadVerifier, token core.LeaseToken) error {
	return verifyPayloadObject(verifier, "lease_token", token, token.SignatureAlgorithm, token.Signature)
}

func SignHandoffOffer(signer PayloadSigner, offer *core.HandoffOffer) error {
	if signer == nil || offer == nil {
		return nil
	}
	return signPayloadObject(signer, "handoff_offer", offer, func() {
		offer.SignatureAlgorithm = signer.Algorithm()
	}, func() string { return offer.Signature }, func(v string) { offer.Signature = v })
}

func VerifyHandoffOffer(verifier PayloadVerifier, offer core.HandoffOffer) error {
	return verifyPayloadObject(verifier, "handoff_offer", offer, offer.SignatureAlgorithm, offer.Signature)
}

func SignHandoffAccept(signer PayloadSigner, accept *core.HandoffAccept) error {
	if signer == nil || accept == nil {
		return nil
	}
	return signPayloadObject(signer, "handoff_accept", accept, func() {
		accept.SignatureAlgorithm = signer.Algorithm()
	}, func() string { return accept.Signature }, func(v string) { accept.Signature = v })
}

func VerifyHandoffAccept(verifier PayloadVerifier, accept core.HandoffAccept) error {
	return verifyPayloadObject(verifier, "handoff_accept", accept, accept.SignatureAlgorithm, accept.Signature)
}

func SignResumeCommit(signer PayloadSigner, commit *core.ResumeCommit) error {
	if signer == nil || commit == nil {
		return nil
	}
	return signPayloadObject(signer, "resume_commit", commit, func() {
		commit.SignatureAlgorithm = signer.Algorithm()
	}, func() string { return commit.Signature }, func(v string) { commit.Signature = v })
}

func VerifyResumeCommit(verifier PayloadVerifier, commit core.ResumeCommit) error {
	return verifyPayloadObject(verifier, "resume_commit", commit, commit.SignatureAlgorithm, commit.Signature)
}

func SignFenceNotice(signer PayloadSigner, notice *core.FenceNotice) error {
	if signer == nil || notice == nil {
		return nil
	}
	return signPayloadObject(signer, "fence_notice", notice, func() {
		notice.SignatureAlgorithm = signer.Algorithm()
	}, func() string { return notice.Signature }, func(v string) { notice.Signature = v })
}

func VerifyFenceNotice(verifier PayloadVerifier, notice core.FenceNotice) error {
	return verifyPayloadObject(verifier, "fence_notice", notice, notice.SignatureAlgorithm, notice.Signature)
}

func SignResumeReceipt(signer PayloadSigner, receipt *core.ResumeReceipt) error {
	if signer == nil || receipt == nil {
		return nil
	}
	return signPayloadObject(signer, "resume_receipt", receipt, func() {
		receipt.SignatureAlgorithm = signer.Algorithm()
	}, func() string { return receipt.Signature }, func(v string) { receipt.Signature = v })
}

func VerifyResumeReceipt(verifier PayloadVerifier, receipt core.ResumeReceipt) error {
	return verifyPayloadObject(verifier, "resume_receipt", receipt, receipt.SignatureAlgorithm, receipt.Signature)
}

func SignRuntimeDescriptor(signer PayloadSigner, descriptor *core.RuntimeDescriptor) error {
	if signer == nil || descriptor == nil {
		return nil
	}
	return signPayloadObject(signer, "runtime_descriptor", descriptor, func() {
		descriptor.SignatureAlgorithm = signer.Algorithm()
	}, func() string { return descriptor.Signature }, func(v string) { descriptor.Signature = v })
}

func VerifyRuntimeDescriptor(verifier PayloadVerifier, descriptor core.RuntimeDescriptor) error {
	return verifyPayloadObject(verifier, "runtime_descriptor", descriptor, descriptor.SignatureAlgorithm, descriptor.Signature)
}

func SignExportDescriptor(signer PayloadSigner, descriptor *core.ExportDescriptor) error {
	if signer == nil || descriptor == nil {
		return nil
	}
	return signPayloadObject(signer, "export_descriptor", descriptor, func() {
		descriptor.SignatureAlgorithm = signer.Algorithm()
	}, func() string { return descriptor.Signature }, func(v string) { descriptor.Signature = v })
}

func VerifyExportDescriptor(verifier PayloadVerifier, descriptor core.ExportDescriptor) error {
	return verifyPayloadObject(verifier, "export_descriptor", descriptor, descriptor.SignatureAlgorithm, descriptor.Signature)
}

func SignTrustBundle(signer PayloadSigner, bundle *core.TrustBundle) error {
	if signer == nil || bundle == nil {
		return nil
	}
	return signPayloadObject(signer, "trust_bundle", bundle, func() {
		bundle.SignatureAlgorithm = signer.Algorithm()
	}, func() string { return bundle.Signature }, func(v string) { bundle.Signature = v })
}

func VerifyTrustBundle(verifier PayloadVerifier, bundle core.TrustBundle) error {
	return verifyPayloadObject(verifier, "trust_bundle", bundle, bundle.SignatureAlgorithm, bundle.Signature)
}

func signPayloadObject[T any](signer PayloadSigner, kind string, value *T, setAlgorithm func(), getSignature func() string, setSignature func(string)) error {
	setAlgorithm()
	setSignature("")
	payload, err := canonicalSignedPayload(kind, *value)
	if err != nil {
		return err
	}
	signature, err := signer.SignPayload(payload)
	if err != nil {
		return err
	}
	setSignature(signature)
	return nil
}

func verifyPayloadObject[T any](verifier PayloadVerifier, kind string, value T, algorithm, signature string) error {
	if verifier == nil {
		return nil
	}
	if strings.TrimSpace(signature) == "" {
		return fmt.Errorf("%s signature required", kind)
	}
	payload, err := canonicalSignedPayload(kind, value)
	if err != nil {
		return err
	}
	return verifier.VerifyPayload(payload, algorithm, signature)
}

func canonicalSignedPayload(kind string, value any) ([]byte, error) {
	normalized, err := clearSignatureFields(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Kind    string `json:"kind"`
		Payload any    `json:"payload"`
	}{
		Kind:    kind,
		Payload: normalized,
	})
}

func clearSignatureFields(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	delete(normalized, "signature")
	delete(normalized, "signature_algorithm")
	return normalized, nil
}
