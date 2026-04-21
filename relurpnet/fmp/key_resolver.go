package fmp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type RecipientKeyRecord struct {
	Recipient string
	KeyID     string
	Version   string
	PublicKey []byte
	Active    bool
	ExpiresAt time.Time
	RevokedAt time.Time
}

type TrustBundleRecipientKeyResolver struct {
	Trust  TrustBundleStore
	Static map[string][][]byte
	Now    func() time.Time
}

func (r *TrustBundleRecipientKeyResolver) ResolveRecipientKeys(ctx context.Context, recipient string) ([]RecipientKeyRecord, error) {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" {
		return nil, fmt.Errorf("recipient required")
	}
	now := time.Now().UTC()
	if r != nil && r.Now != nil {
		now = r.Now().UTC()
	}
	var records []RecipientKeyRecord
	if r != nil && r.Trust != nil {
		trustDomain := trustDomainForRecipient(recipient)
		if trustDomain != "" {
			bundle, err := r.Trust.GetTrustBundle(ctx, trustDomain)
			if err != nil {
				return nil, err
			}
			if bundle != nil {
				records = append(records, activeRecipientKeysFromBundle(*bundle, recipient, now)...)
			}
		}
	}
	if len(records) == 0 && r != nil && len(r.Static) > 0 {
		for i, key := range r.Static[recipient] {
			if len(key) == 0 {
				continue
			}
			records = append(records, RecipientKeyRecord{
				Recipient: recipient,
				KeyID:     fmt.Sprintf("static-%d", i+1),
				PublicKey: append([]byte(nil), key...),
				Active:    true,
			})
		}
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("recipient key for %s not found", recipient)
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Active != records[j].Active {
			return records[i].Active
		}
		if !records[i].ExpiresAt.Equal(records[j].ExpiresAt) {
			return records[i].ExpiresAt.After(records[j].ExpiresAt)
		}
		if records[i].Version != records[j].Version {
			return records[i].Version > records[j].Version
		}
		return records[i].KeyID > records[j].KeyID
	})
	return records, nil
}

func activeRecipientKeysFromBundle(bundle core.TrustBundle, recipient string, now time.Time) []RecipientKeyRecord {
	out := make([]RecipientKeyRecord, 0, len(bundle.RecipientKeys))
	for _, key := range bundle.RecipientKeys {
		if !strings.EqualFold(strings.TrimSpace(key.Recipient), recipient) {
			continue
		}
		if len(key.PublicKey) == 0 {
			continue
		}
		if !key.RevokedAt.IsZero() && !key.RevokedAt.After(now) {
			continue
		}
		if !key.ExpiresAt.IsZero() && !key.ExpiresAt.After(now) {
			continue
		}
		out = append(out, RecipientKeyRecord{
			Recipient: recipient,
			KeyID:     strings.TrimSpace(key.KeyID),
			Version:   strings.TrimSpace(key.Version),
			PublicKey: append([]byte(nil), key.PublicKey...),
			Active:    key.Active || key.RevokedAt.IsZero(),
			ExpiresAt: key.ExpiresAt,
			RevokedAt: key.RevokedAt,
		})
	}
	return out
}

func trustDomainForRecipient(recipient string) string {
	recipient = strings.TrimSpace(recipient)
	if strings.HasPrefix(recipient, "runtime://") {
		trimmed := strings.TrimPrefix(recipient, "runtime://")
		if idx := strings.Index(trimmed, "/"); idx > 0 {
			return strings.TrimSpace(trimmed[:idx])
		}
	}
	if strings.HasPrefix(recipient, "gateway://") {
		trimmed := strings.TrimPrefix(recipient, "gateway://")
		if idx := strings.Index(trimmed, "/"); idx > 0 {
			return strings.TrimSpace(trimmed[:idx])
		}
	}
	return ""
}
