package admin

import (
	"context"
	"os"
	"testing"
	"time"

	nexuscfg "codeburg.org/lexbit/relurpify/app/nexus/config"
	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkmanifest "codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/relurpnet/node"
	"gopkg.in/yaml.v3"
)

func TestApproveAndRejectPairing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	workspace := t.TempDir()
	paths := frameworkmanifest.New(workspace)
	if err := os.MkdirAll(paths.ConfigRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(nexuscfg.Config{
		Gateway: nexuscfg.GatewayConfig{Bind: ":9999", Path: "/gateway"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.NexusConfigFile(), data, 0o644); err != nil {
		t.Fatal(err)
	}

	nodeStore, err := db.NewSQLiteNodeStore(paths.NodesFile())
	if err != nil {
		t.Fatal(err)
	}
	if err := nodeStore.SavePendingPairing(ctx, node.PendingPairing{
		Code: "approve-me",
		Cred: node.NodeCredential{
			DeviceID:  "device-1",
			PublicKey: []byte("pk"),
			IssuedAt:  time.Now().UTC(),
		},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := nodeStore.SavePendingPairing(ctx, node.PendingPairing{
		Code: "reject-me",
		Cred: node.NodeCredential{
			DeviceID:  "device-2",
			PublicKey: []byte("pk2"),
			IssuedAt:  time.Now().UTC(),
		},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	_ = nodeStore.Close()

	if err := ApprovePairing(ctx, workspace, "", "approve-me"); err != nil {
		t.Fatal(err)
	}
	if err := RejectPairing(ctx, workspace, "", "reject-me"); err != nil {
		t.Fatal(err)
	}

	nodeStore, err = db.NewSQLiteNodeStore(paths.NodesFile())
	if err != nil {
		t.Fatal(err)
	}
	defer nodeStore.Close()
	if pairing, err := nodeStore.GetPendingPairing(ctx, "approve-me"); err != nil || pairing != nil {
		t.Fatalf("approve pending pairing = %+v, err=%v", pairing, err)
	}
	if pairing, err := nodeStore.GetPendingPairing(ctx, "reject-me"); err != nil || pairing != nil {
		t.Fatalf("reject pending pairing = %+v, err=%v", pairing, err)
	}
	nodeDesc, err := nodeStore.GetNode(ctx, "device-1")
	if err != nil {
		t.Fatal(err)
	}
	if nodeDesc == nil || nodeDesc.ID != "device-1" {
		t.Fatalf("approved node = %+v", nodeDesc)
	}
	if nodeDesc.TenantID != "local" || nodeDesc.TrustClass != core.TrustClassRemoteApproved || nodeDesc.Owner.ID != "device-1" {
		t.Fatalf("approved node metadata = %+v", nodeDesc)
	}
	cred, err := nodeStore.GetCredential(ctx, "device-1")
	if err != nil {
		t.Fatal(err)
	}
	if cred == nil || cred.DeviceID != "device-1" {
		t.Fatalf("approved credential = %+v", cred)
	}

	identityStore, err := db.NewSQLiteIdentityStore(paths.IdentityStoreFile())
	if err != nil {
		t.Fatal(err)
	}
	defer identityStore.Close()
	enrollment, err := identityStore.GetNodeEnrollment(ctx, "local", "device-1")
	if err != nil {
		t.Fatal(err)
	}
	if enrollment == nil || enrollment.NodeID != "device-1" {
		t.Fatalf("approved enrollment = %+v", enrollment)
	}
	tenant, err := identityStore.GetTenant(ctx, "local")
	if err != nil {
		t.Fatal(err)
	}
	if tenant == nil || tenant.ID != "local" {
		t.Fatalf("approved tenant = %+v", tenant)
	}
	subject, err := identityStore.GetSubject(ctx, "local", core.SubjectKindNode, "device-1")
	if err != nil {
		t.Fatal(err)
	}
	if subject == nil || subject.ID != "device-1" {
		t.Fatalf("approved subject = %+v", subject)
	}
}
