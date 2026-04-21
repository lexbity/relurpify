package fmp

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestMediationControllerUnsealsAndResealsForDestinationRuntime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	gatewayRecipient := QualifiedGatewayRecipient("mesh.remote", "node-gw")
	runtimeRecipient := qualifiedRuntimeName("mesh.remote", "rt-remote")
	resolver := StaticRecipientKeyResolver{
		gatewayRecipient: []byte("0123456789abcdef0123456789abcdef"),
		runtimeRecipient: []byte("abcdef0123456789abcdef0123456789"),
	}
	packager := JSONPackager{
		RuntimeStore: fakeWorkflowRuntimeStore{},
		KeyResolver:  resolver,
	}
	lineage := core.LineageRecord{
		LineageID:        "lineage-1",
		TenantID:         "tenant-1",
		TaskClass:        "agent.run",
		ContextClass:     "workflow-runtime",
		SensitivityClass: core.SensitivityClassModerate,
		Owner:            core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-1",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-local",
		State:     core.AttemptStateRunning,
	}
	pkg, err := packager.BuildPackage(context.Background(), lineage, attempt, RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	sealed, err := packager.SealPackage(context.Background(), pkg.Manifest, pkg, []string{gatewayRecipient})
	if err != nil {
		t.Fatalf("SealPackage() error = %v", err)
	}
	discovery := &InMemoryDiscoveryStore{}
	if err := discovery.UpsertRuntimeAdvertisement(context.Background(), core.RuntimeAdvertisement{
		TrustDomain: "mesh.remote",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:      "rt-remote",
			NodeID:         "node-remote",
			RuntimeVersion: "1.0.0",
		},
	}); err != nil {
		t.Fatalf("UpsertRuntimeAdvertisement() error = %v", err)
	}
	if err := discovery.UpsertExportAdvertisement(context.Background(), core.ExportAdvertisement{
		TrustDomain: "mesh.remote",
		RuntimeID:   "rt-remote",
		NodeID:      "node-remote",
		Export: core.ExportDescriptor{
			ExportName: "agent.resume",
			RouteMode:  core.RouteModeMediated,
		},
	}); err != nil {
		t.Fatalf("UpsertExportAdvertisement() error = %v", err)
	}
	audit := core.NewInMemoryAuditLogger(16)
	svc := &Service{
		Discovery: discovery,
		Audit:     audit,
	}
	mediator := &MediationController{
		Packager:       packager,
		LocalRecipient: gatewayRecipient,
		Now:            func() time.Time { return now },
	}
	req, refusal, err := mediator.MediateForward(context.Background(), svc, core.GatewayForwardRequest{
		TenantID:           "tenant-1",
		LineageID:          lineage.LineageID,
		TrustDomain:        "mesh.remote",
		SourceDomain:       "mesh.remote",
		GatewayIdentity:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"},
		DestinationExport:  QualifiedExportName("mesh.remote", "agent.resume"),
		RouteMode:          core.RouteModeMediated,
		MediationRequested: true,
		SizeBytes:          pkg.Manifest.SizeBytes,
		ContextManifestRef: pkg.Manifest.ContextID,
		SealedContext:      *sealed,
	})
	if err != nil {
		t.Fatalf("MediateForward() error = %v", err)
	}
	if refusal != nil {
		t.Fatalf("MediateForward() refusal = %+v", refusal)
	}
	if len(req.SealedContext.RecipientBindings) != 1 || req.SealedContext.RecipientBindings[0] != runtimeRecipient {
		t.Fatalf("recipient bindings = %+v, want %s", req.SealedContext.RecipientBindings, runtimeRecipient)
	}
	var out PortableContextPackage
	if err := (JSONPackager{KeyResolver: resolver, LocalRecipient: runtimeRecipient}).UnsealPackage(context.Background(), req.SealedContext, &out); err != nil {
		t.Fatalf("UnsealPackage(runtime) error = %v", err)
	}
	if string(out.ExecutionPayload) != string(pkg.ExecutionPayload) {
		t.Fatal("mediated payload mismatch")
	}
	records, err := audit.Query(context.Background(), core.AuditQuery{Type: "fmp.federation.mediated"})
	if err != nil {
		t.Fatalf("audit.Query() error = %v", err)
	}
	if len(records) != 1 || records[0].Result != "ok" {
		t.Fatalf("unexpected mediation audit = %+v", records)
	}
}
