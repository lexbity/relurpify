package fmp

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// ContextPackager is part of the Phase 1 frozen FMP surface.
// Later phases should replace the concrete packaging and crypto behavior behind
// this interface before expanding the interface itself.
type ContextPackager interface {
	BuildPackage(ctx context.Context, lineage core.LineageRecord, attempt core.AttemptRecord, query RuntimeQuery) (*PortableContextPackage, error)
	SealPackage(ctx context.Context, manifest core.ContextManifest, pkg *PortableContextPackage, recipients []string) (*core.SealedContext, error)
	UnsealPackage(ctx context.Context, sealed core.SealedContext, pkg *PortableContextPackage) error
}

type RuntimeQuery struct {
	WorkflowID string
	RunID      string
	TaskID     string
	TTL        time.Duration
}

// RuntimeSnapshotStore is the current packaging input seam.
// It remains intentionally minimal until the runtime-executed continuation
// path replaces the current scaffolded export/import flow.
type RuntimeSnapshotStore interface {
	QueryWorkflowRuntime(ctx context.Context, workflowID, runID string) (map[string]any, error)
}

type RecipientKeyResolver interface {
	ResolveRecipientKey(ctx context.Context, recipient string) ([]byte, error)
}

type EncryptedObjectStore interface {
	PutObject(ctx context.Context, ref string, ciphertext []byte, expiresAt time.Time) error
	GetObject(ctx context.Context, ref string) ([]byte, error)
}

type InMemoryEncryptedObjectStore struct {
	mu      sync.Mutex
	objects map[string]storedEncryptedObject
	Now     func() time.Time
}

type storedEncryptedObject struct {
	ciphertext []byte
	expiresAt  time.Time
}

type StaticRecipientKeyResolver map[string][]byte

func (r StaticRecipientKeyResolver) ResolveRecipientKey(_ context.Context, recipient string) ([]byte, error) {
	key, ok := r[strings.TrimSpace(recipient)]
	if !ok {
		return nil, fmt.Errorf("recipient key for %s not found", recipient)
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("recipient key for %s empty", recipient)
	}
	out := make([]byte, len(key))
	copy(out, key)
	return out, nil
}

type JSONPackager struct {
	RuntimeStore      RuntimeSnapshotStore
	KeyResolver       RecipientKeyResolver
	ObjectStore       EncryptedObjectStore
	DefaultRecipients []string
	LocalRecipient    string
	Random            io.Reader
	CipherSuite       string
	InlineThreshold   int
	ChunkSize         int
	ExternalThreshold int
}

func (p JSONPackager) BuildPackage(ctx context.Context, lineage core.LineageRecord, attempt core.AttemptRecord, query RuntimeQuery) (*PortableContextPackage, error) {
	if p.RuntimeStore == nil {
		return nil, fmt.Errorf("workflow runtime store unavailable")
	}
	payload, err := p.RuntimeStore.QueryWorkflowRuntime(ctx, query.WorkflowID, query.RunID)
	if err != nil {
		return nil, err
	}
	execution, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(execution)
	transferMode := p.transferModeForSize(len(execution))
	chunkCount := chunkCountForSize(len(execution), p.chunkSize())
	manifest := core.ContextManifest{
		ContextID:        lineage.LineageID + ":" + attempt.AttemptID,
		LineageID:        lineage.LineageID,
		AttemptID:        attempt.AttemptID,
		ContextClass:     lineage.ContextClass,
		SchemaVersion:    "fmp.context.v1",
		SizeBytes:        int64(len(execution)),
		ChunkCount:       chunkCount,
		ContentHash:      hex.EncodeToString(sum[:]),
		SensitivityClass: lineage.SensitivityClass,
		TTL:              query.TTL,
		TransferMode:     transferMode,
		EncryptionMode:   core.EncryptionModeEndToEnd,
		CreationTime:     time.Now().UTC(),
	}
	return &PortableContextPackage{
		Manifest:         manifest,
		ExecutionPayload: execution,
	}, manifest.Validate()
}

func (p JSONPackager) SealPackage(ctx context.Context, manifest core.ContextManifest, pkg *PortableContextPackage, recipients []string) (*core.SealedContext, error) {
	if pkg == nil {
		return nil, fmt.Errorf("portable package required")
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	resolver := p.KeyResolver
	if resolver == nil {
		return nil, fmt.Errorf("recipient key resolver unavailable")
	}
	resolvedRecipients := normalizedRecipients(recipients)
	if len(resolvedRecipients) == 0 {
		resolvedRecipients = normalizedRecipients(p.DefaultRecipients)
	}
	if len(resolvedRecipients) == 0 {
		return nil, fmt.Errorf("recipient bindings required")
	}
	payload, err := marshalSealedPayload(pkg)
	if err != nil {
		return nil, err
	}
	dek, err := randomBytes(p.randomSource(), 32)
	if err != nil {
		return nil, err
	}
	payloadNonce, payloadCiphertext, err := encryptAEAD(dek, payload, payloadAAD(manifest, resolvedRecipients))
	if err != nil {
		return nil, err
	}
	ciphertextChunks := splitCiphertext(payloadCiphertext, p.chunkSize())
	externalRefs := []string(nil)
	switch manifest.TransferMode {
	case core.TransferModeInline:
		// keep inline ciphertext as-is
	case core.TransferModeChunked:
		if len(ciphertextChunks) == 0 {
			ciphertextChunks = [][]byte{payloadCiphertext}
		}
	case core.TransferModeExternal:
		if p.ObjectStore == nil {
			return nil, fmt.Errorf("encrypted object store unavailable for external transfer")
		}
		externalRefs = externalObjectRefs(manifest.ContextID, len(ciphertextChunks))
		for i, ref := range externalRefs {
			if err := p.ObjectStore.PutObject(ctx, ref, ciphertextChunks[i], manifestExpiry(manifest)); err != nil {
				return nil, err
			}
		}
		pkg.Manifest.ObjectRefs = append([]string(nil), externalRefs...)
		pkg.Manifest.ChunkCount = len(externalRefs)
		ciphertextChunks = nil
	default:
		return nil, fmt.Errorf("unsupported transfer mode %q", manifest.TransferMode)
	}
	wrappedKeys := make(map[string]map[string]string, len(resolvedRecipients))
	for _, recipient := range resolvedRecipients {
		recipientKey, err := resolver.ResolveRecipientKey(context.Background(), recipient)
		if err != nil {
			return nil, err
		}
		wrapNonce, wrappedDEK, err := encryptAEAD(normalizeKey(recipientKey), dek, wrapAAD(manifest.ContextID, recipient))
		if err != nil {
			return nil, err
		}
		wrappedKeys[recipient] = map[string]string{
			"nonce":      base64.RawStdEncoding.EncodeToString(wrapNonce),
			"ciphertext": base64.RawStdEncoding.EncodeToString(wrappedDEK),
		}
	}
	sum := sha256.Sum256(payloadCiphertext)
	sealed := &core.SealedContext{
		EnvelopeVersion:    "fmp.sealed.v1",
		ContextManifestRef: manifest.ContextID,
		CipherSuite:        p.cipherSuite(),
		RecipientBindings:  append([]string(nil), resolvedRecipients...),
		CiphertextChunks:   ciphertextChunks,
		ExternalObjectRefs: externalRefs,
		IntegrityTag:       hex.EncodeToString(sum[:]),
		ReplayProtectionData: map[string]any{
			"lineage_id":    manifest.LineageID,
			"attempt_id":    manifest.AttemptID,
			"payload_nonce": base64.RawStdEncoding.EncodeToString(payloadNonce),
			"wrapped_keys":  wrappedKeys,
			"transfer_mode": string(manifest.TransferMode),
			"chunk_count":   len(externalRefs) + len(ciphertextChunks),
		},
	}
	return sealed, sealed.Validate()
}

func (p JSONPackager) UnsealPackage(ctx context.Context, sealed core.SealedContext, pkg *PortableContextPackage) error {
	if err := sealed.Validate(); err != nil {
		return err
	}
	if pkg == nil {
		return fmt.Errorf("portable package required")
	}
	if p.KeyResolver == nil {
		return fmt.Errorf("recipient key resolver unavailable")
	}
	payloadCiphertext, err := p.loadCiphertext(ctx, sealed)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payloadCiphertext)
	if hex.EncodeToString(sum[:]) != sealed.IntegrityTag {
		return fmt.Errorf("sealed context integrity tag mismatch")
	}
	payloadNonce, err := replayBytes(sealed.ReplayProtectionData, "payload_nonce")
	if err != nil {
		return err
	}
	wrappedKeys, err := parseWrappedKeys(sealed.ReplayProtectionData)
	if err != nil {
		return err
	}
	recipients := normalizedRecipients(sealed.RecipientBindings)
	candidateRecipients := recipients
	if strings.TrimSpace(p.LocalRecipient) != "" {
		candidateRecipients = []string{strings.TrimSpace(p.LocalRecipient)}
	}
	var payload []byte
	for _, recipient := range candidateRecipients {
		entry, ok := wrappedKeys[recipient]
		if !ok {
			continue
		}
		recipientKey, err := p.KeyResolver.ResolveRecipientKey(ctx, recipient)
		if err != nil {
			continue
		}
		wrapNonce, err := base64.RawStdEncoding.DecodeString(entry["nonce"])
		if err != nil {
			continue
		}
		wrappedDEK, err := base64.RawStdEncoding.DecodeString(entry["ciphertext"])
		if err != nil {
			continue
		}
		dek, err := decryptAEAD(normalizeKey(recipientKey), wrapNonce, wrappedDEK, wrapAAD(sealed.ContextManifestRef, recipient))
		if err != nil {
			continue
		}
		payload, err = decryptAEAD(dek, payloadNonce, payloadCiphertext, payloadAAD(unsealManifestAAD(sealed), recipients))
		if err == nil {
			break
		}
	}
	if len(payload) == 0 {
		return fmt.Errorf("unable to unwrap sealed context for available recipient")
	}
	sealedPayload := sealedPackagePayload{}
	if err := json.Unmarshal(payload, &sealedPayload); err != nil {
		return err
	}
	pkg.ExecutionPayload = append([]byte(nil), sealedPayload.ExecutionPayload...)
	pkg.DeclarativeMemory = append([]byte(nil), sealedPayload.DeclarativeMemory...)
	pkg.ProceduralMemory = append([]byte(nil), sealedPayload.ProceduralMemory...)
	pkg.RetrievalReferences = append([]string(nil), sealedPayload.RetrievalReferences...)
	pkg.AdditionalObjectRefs = append([]string(nil), sealedPayload.AdditionalObjectRefs...)
	return nil
}

type sealedPackagePayload struct {
	ExecutionPayload     []byte   `json:"execution_payload,omitempty"`
	DeclarativeMemory    []byte   `json:"declarative_memory,omitempty"`
	ProceduralMemory     []byte   `json:"procedural_memory,omitempty"`
	RetrievalReferences  []string `json:"retrieval_references,omitempty"`
	AdditionalObjectRefs []string `json:"additional_object_refs,omitempty"`
}

func marshalSealedPayload(pkg *PortableContextPackage) ([]byte, error) {
	return json.Marshal(sealedPackagePayload{
		ExecutionPayload:     append([]byte(nil), pkg.ExecutionPayload...),
		DeclarativeMemory:    append([]byte(nil), pkg.DeclarativeMemory...),
		ProceduralMemory:     append([]byte(nil), pkg.ProceduralMemory...),
		RetrievalReferences:  append([]string(nil), pkg.RetrievalReferences...),
		AdditionalObjectRefs: append([]string(nil), pkg.AdditionalObjectRefs...),
	})
}

func parseWrappedKeys(data map[string]any) (map[string]map[string]string, error) {
	raw, ok := data["wrapped_keys"]
	if !ok {
		return nil, fmt.Errorf("sealed context missing wrapped keys")
	}
	typed, ok := raw.(map[string]map[string]string)
	if ok {
		return typed, nil
	}
	intermediate, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("sealed context wrapped keys invalid")
	}
	out := make(map[string]map[string]string, len(intermediate))
	for recipient, value := range intermediate {
		record, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("sealed context wrapped key record invalid")
		}
		nonce, _ := record["nonce"].(string)
		ciphertext, _ := record["ciphertext"].(string)
		if nonce == "" || ciphertext == "" {
			return nil, fmt.Errorf("sealed context wrapped key record incomplete")
		}
		out[recipient] = map[string]string{
			"nonce":      nonce,
			"ciphertext": ciphertext,
		}
	}
	return out, nil
}

func replayBytes(data map[string]any, key string) ([]byte, error) {
	raw, ok := data[key]
	if !ok {
		return nil, fmt.Errorf("sealed context missing %s", key)
	}
	text, ok := raw.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("sealed context %s invalid", key)
	}
	return base64.RawStdEncoding.DecodeString(text)
}

func (p JSONPackager) loadCiphertext(ctx context.Context, sealed core.SealedContext) ([]byte, error) {
	switch {
	case len(sealed.CiphertextChunks) > 0:
		return joinCiphertext(sealed.CiphertextChunks), nil
	case len(sealed.ExternalObjectRefs) > 0:
		if p.ObjectStore == nil {
			return nil, fmt.Errorf("encrypted object store unavailable for external object refs")
		}
		chunks := make([][]byte, 0, len(sealed.ExternalObjectRefs))
		for _, ref := range sealed.ExternalObjectRefs {
			chunk, err := p.ObjectStore.GetObject(ctx, ref)
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, chunk)
		}
		return joinCiphertext(chunks), nil
	default:
		return nil, fmt.Errorf("sealed context missing ciphertext")
	}
}

func payloadAAD(manifest core.ContextManifest, recipients []string) []byte {
	sum := sha256.Sum256([]byte(manifest.ContextID + "|" + manifest.LineageID + "|" + manifest.AttemptID + "|" + strings.Join(recipients, ",")))
	return sum[:]
}

func unsealManifestAAD(sealed core.SealedContext) core.ContextManifest {
	manifest := core.ContextManifest{ContextID: sealed.ContextManifestRef}
	if lineageID, _ := sealed.ReplayProtectionData["lineage_id"].(string); lineageID != "" {
		manifest.LineageID = lineageID
	}
	if attemptID, _ := sealed.ReplayProtectionData["attempt_id"].(string); attemptID != "" {
		manifest.AttemptID = attemptID
	}
	return manifest
}

func wrapAAD(contextID, recipient string) []byte {
	sum := sha256.Sum256([]byte(contextID + "|" + recipient))
	return sum[:]
}

func (s *InMemoryEncryptedObjectStore) PutObject(_ context.Context, ref string, ciphertext []byte, expiresAt time.Time) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("object ref required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.objects == nil {
		s.objects = map[string]storedEncryptedObject{}
	}
	s.objects[ref] = storedEncryptedObject{
		ciphertext: append([]byte(nil), ciphertext...),
		expiresAt:  expiresAt.UTC(),
	}
	return nil
}

func (s *InMemoryEncryptedObjectStore) GetObject(_ context.Context, ref string) ([]byte, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, fmt.Errorf("object ref required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.objects == nil {
		return nil, fmt.Errorf("object %s not found", ref)
	}
	sweepAt := time.Now().UTC()
	if s.Now != nil {
		sweepAt = s.Now().UTC()
	}
	for key, object := range s.objects {
		if !object.expiresAt.IsZero() && sweepAt.After(object.expiresAt) {
			delete(s.objects, key)
		}
	}
	object, ok := s.objects[ref]
	if !ok {
		return nil, fmt.Errorf("object %s not found", ref)
	}
	return append([]byte(nil), object.ciphertext...), nil
}

func (p JSONPackager) transferModeForSize(size int) core.TransferMode {
	switch {
	case p.ExternalThreshold > 0 && size >= p.ExternalThreshold:
		return core.TransferModeExternal
	case size > p.inlineThreshold():
		return core.TransferModeChunked
	default:
		return core.TransferModeInline
	}
}

func (p JSONPackager) inlineThreshold() int {
	if p.InlineThreshold > 0 {
		return p.InlineThreshold
	}
	return 64 * 1024
}

func (p JSONPackager) chunkSize() int {
	if p.ChunkSize > 0 {
		return p.ChunkSize
	}
	return 32 * 1024
}

func chunkCountForSize(size, chunkSize int) int {
	if size <= 0 {
		return 1
	}
	if chunkSize <= 0 {
		chunkSize = 32 * 1024
	}
	count := size / chunkSize
	if size%chunkSize != 0 {
		count++
	}
	if count == 0 {
		return 1
	}
	return count
}

func splitCiphertext(ciphertext []byte, chunkSize int) [][]byte {
	if len(ciphertext) == 0 {
		return nil
	}
	if chunkSize <= 0 || len(ciphertext) <= chunkSize {
		return [][]byte{append([]byte(nil), ciphertext...)}
	}
	out := make([][]byte, 0, chunkCountForSize(len(ciphertext), chunkSize))
	for start := 0; start < len(ciphertext); start += chunkSize {
		end := start + chunkSize
		if end > len(ciphertext) {
			end = len(ciphertext)
		}
		out = append(out, append([]byte(nil), ciphertext[start:end]...))
	}
	return out
}

func joinCiphertext(chunks [][]byte) []byte {
	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
	}
	out := make([]byte, 0, total)
	for _, chunk := range chunks {
		out = append(out, chunk...)
	}
	return out
}

func externalObjectRefs(contextID string, count int) []string {
	if count <= 0 {
		count = 1
	}
	refs := make([]string, 0, count)
	base := strings.ReplaceAll(strings.TrimSpace(contextID), ":", "/")
	for i := 0; i < count; i++ {
		refs = append(refs, fmt.Sprintf("fmp/%s/chunk-%03d", base, i))
	}
	return refs
}

func manifestExpiry(manifest core.ContextManifest) time.Time {
	if manifest.TTL <= 0 {
		return manifest.CreationTime.UTC().Add(15 * time.Minute)
	}
	return manifest.CreationTime.UTC().Add(manifest.TTL)
}

func encryptAEAD(key, plaintext, aad []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(normalizeKey(key))
	if err != nil {
		return nil, nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce, err := randomBytes(rand.Reader, aead.NonceSize())
	if err != nil {
		return nil, nil, err
	}
	return nonce, aead.Seal(nil, nonce, plaintext, aad), nil
}

func decryptAEAD(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(normalizeKey(key))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ciphertext, aad)
}

func normalizeKey(key []byte) []byte {
	sum := sha256.Sum256(key)
	return sum[:]
}

func normalizedRecipients(recipients []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		trimmed := strings.TrimSpace(recipient)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func randomBytes(r io.Reader, size int) ([]byte, error) {
	out := make([]byte, size)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p JSONPackager) randomSource() io.Reader {
	if p.Random != nil {
		return p.Random
	}
	return rand.Reader
}

func (p JSONPackager) cipherSuite() string {
	if strings.TrimSpace(p.CipherSuite) != "" {
		return strings.TrimSpace(p.CipherSuite)
	}
	return "aes256-gcm+aes256-gcm-wrap.v1"
}
