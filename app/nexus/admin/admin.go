package admin

import (
	"context"
	"fmt"

	nexusbootstrap "codeburg.org/lexbit/relurpify/app/nexus/bootstrap"
	nexuscfg "codeburg.org/lexbit/relurpify/app/nexus/config"
	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/framework/core"
	fwnode "codeburg.org/lexbit/relurpify/relurpnet/node"
)

func ResolveConfig(workspace, configPath string) (config.Paths, nexuscfg.Config, error) {
	return nexusbootstrap.ResolveConfig(workspace, configPath)
}

func ApprovePairing(ctx context.Context, workspace, configPath, pairingCode string) error {
	paths, cfg, err := ResolveConfig(workspace, configPath)
	if err != nil {
		return err
	}
	identityStore, err := db.NewSQLiteIdentityStore(paths.IdentityStoreFile())
	if err != nil {
		return err
	}
	defer identityStore.Close()
	manager, store, logStore, err := nexusbootstrap.OpenNodeManager(paths, cfg)
	if err != nil {
		return err
	}
	defer store.Close()
	defer logStore.Close()

	pairing, _, _ := manager.PairingStatus(ctx, pairingCode)
	if err := manager.ApprovePairing(ctx, pairingCode); err != nil {
		return err
	}
	if pairing != nil {
		enrollment := nodeEnrollmentFromPairing(*pairing)
		if err := upsertTenantAndSubject(ctx, identityStore, enrollment.TenantID, enrollment.Owner.Kind, enrollment.Owner.ID, enrollment.Owner.ID, nil, enrollment.PairedAt); err != nil {
			return err
		}
		if err := identityStore.UpsertNodeEnrollment(ctx, enrollment); err != nil {
			return err
		}
		if err := store.UpsertNode(ctx, nodeDescriptorFromEnrollment(enrollment)); err != nil {
			return err
		}
	}
	return nil
}

func RejectPairing(ctx context.Context, workspace, configPath, pairingCode string) error {
	paths, cfg, err := ResolveConfig(workspace, configPath)
	if err != nil {
		return err
	}
	manager, store, logStore, err := nexusbootstrap.OpenNodeManager(paths, cfg)
	if err != nil {
		return err
	}
	defer store.Close()
	defer logStore.Close()
	return manager.RejectPairing(ctx, pairingCode)
}

func nodeEnrollmentFromPairing(pairing fwnode.PendingPairing) core.NodeEnrollment {
	tenantID := pairing.Cred.TenantID
	if tenantID == "" {
		tenantID = "local"
	}
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

func SummaryMessage(action, pairingCode string, err error) string {
	if err != nil {
		return fmt.Sprintf("%s %s failed: %v", action, pairingCode, err)
	}
	return fmt.Sprintf("%s %s completed", action, pairingCode)
}
