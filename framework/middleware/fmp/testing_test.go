package fmp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
)

type memoryEventLog struct {
	events []core.FrameworkEvent
}

func (m *memoryEventLog) Append(_ context.Context, _ string, events []core.FrameworkEvent) ([]uint64, error) {
	for i := range events {
		events[i].Seq = uint64(len(m.events) + 1)
		m.events = append(m.events, events[i])
	}
	return nil, nil
}

func (m *memoryEventLog) Read(_ context.Context, _ string, _ uint64, _ int, _ bool) ([]core.FrameworkEvent, error) {
	return append([]core.FrameworkEvent(nil), m.events...), nil
}

func (m *memoryEventLog) ReadByType(_ context.Context, _ string, _ string, _ uint64, _ int) ([]core.FrameworkEvent, error) {
	return append([]core.FrameworkEvent(nil), m.events...), nil
}

func (m *memoryEventLog) LastSeq(_ context.Context, _ string) (uint64, error) {
	return uint64(len(m.events)), nil
}
func (m *memoryEventLog) TakeSnapshot(_ context.Context, _ string, _ uint64, _ []byte) error {
	return nil
}
func (m *memoryEventLog) LoadSnapshot(_ context.Context, _ string) (uint64, []byte, error) {
	return 0, nil, nil
}
func (m *memoryEventLog) Close() error { return nil }

var _ event.Log = (*memoryEventLog)(nil)
var _ RuntimeEndpoint = (*fakeRuntimeEndpoint)(nil)

type allowPolicy struct{}

func (allowPolicy) EvaluateResume(context.Context, core.LineageRecord, core.HandoffOffer, core.ExportDescriptor) (core.PolicyDecision, error) {
	return core.PolicyDecisionAllow("ok"), nil
}

type denyPolicy struct{}

func (denyPolicy) EvaluateResume(context.Context, core.LineageRecord, core.HandoffOffer, core.ExportDescriptor) (core.PolicyDecision, error) {
	return core.PolicyDecisionDeny("denied"), nil
}

type allowResumePolicy struct{}

func (allowResumePolicy) EvaluateResume(context.Context, ResumePolicyRequest) (core.PolicyDecision, error) {
	return core.PolicyDecisionAllow("ok"), nil
}

type denyResumePolicy struct{}

func (denyResumePolicy) EvaluateResume(context.Context, ResumePolicyRequest) (core.PolicyDecision, error) {
	return core.PolicyDecisionDeny("denied"), nil
}

type delegatedOnlyResumePolicy struct{}

func (delegatedOnlyResumePolicy) EvaluateResume(_ context.Context, req ResumePolicyRequest) (core.PolicyDecision, error) {
	if req.IsDelegated {
		return core.PolicyDecisionAllow("delegated"), nil
	}
	return core.PolicyDecisionDeny("delegation required"), nil
}

type fakeRuntimeEndpoint struct {
	descriptor       core.RuntimeDescriptor
	exportPackage    *PortableContextPackage
	importPackage    *PortableContextPackage
	createdAttempt   *core.AttemptRecord
	issuedReceipt    *core.ResumeReceipt
	validateErr      error
	importErr        error
	createAttemptErr error
	receiptErr       error
	fenceErr         error
	fenceNotices     []core.FenceNotice
	validateCalls    int
	importCalls      int
	createCalls      int
	receiptCalls     int
}

func (f *fakeRuntimeEndpoint) Descriptor(context.Context) (core.RuntimeDescriptor, error) {
	return f.descriptor, nil
}

func (f *fakeRuntimeEndpoint) ExportContext(context.Context, core.LineageRecord, core.AttemptRecord) (*PortableContextPackage, error) {
	return f.exportPackage, nil
}

func (f *fakeRuntimeEndpoint) ValidateContext(context.Context, core.ContextManifest, core.SealedContext) error {
	f.validateCalls++
	return f.validateErr
}

func (f *fakeRuntimeEndpoint) ImportContext(context.Context, core.LineageRecord, core.ContextManifest, core.SealedContext) (*PortableContextPackage, error) {
	f.importCalls++
	if f.importErr != nil {
		return nil, f.importErr
	}
	if f.importPackage != nil {
		return f.importPackage, nil
	}
	return &PortableContextPackage{}, nil
}

func (f *fakeRuntimeEndpoint) CreateAttempt(context.Context, core.LineageRecord, core.HandoffAccept, *PortableContextPackage) (*core.AttemptRecord, error) {
	f.createCalls++
	if f.createAttemptErr != nil {
		return nil, f.createAttemptErr
	}
	if f.createdAttempt != nil {
		return f.createdAttempt, nil
	}
	return &core.AttemptRecord{}, nil
}

func (f *fakeRuntimeEndpoint) FenceAttempt(_ context.Context, notice core.FenceNotice) error {
	f.fenceNotices = append(f.fenceNotices, notice)
	return f.fenceErr
}

func (f *fakeRuntimeEndpoint) IssueReceipt(context.Context, core.LineageRecord, core.AttemptRecord, *PortableContextPackage) (*core.ResumeReceipt, error) {
	f.receiptCalls++
	if f.receiptErr != nil {
		return nil, f.receiptErr
	}
	if f.issuedReceipt != nil {
		return f.issuedReceipt, nil
	}
	return &core.ResumeReceipt{}, nil
}

type fakeTenantLookup struct {
	tenants map[string]core.TenantRecord
}

func (f fakeTenantLookup) GetTenant(_ context.Context, tenantID string) (*core.TenantRecord, error) {
	record, ok := f.tenants[tenantID]
	if !ok {
		return nil, nil
	}
	copy := record
	return &copy, nil
}

type fakeSubjectLookup struct {
	subjects map[string]core.SubjectRecord
}

func (f fakeSubjectLookup) GetSubject(_ context.Context, tenantID string, kind core.SubjectKind, subjectID string) (*core.SubjectRecord, error) {
	record, ok := f.subjects[tenantID+":"+string(kind)+":"+subjectID]
	if !ok {
		return nil, nil
	}
	copy := record
	return &copy, nil
}

type fakeNodeLookup struct {
	enrollments map[string]core.NodeEnrollment
}

func (f fakeNodeLookup) GetNodeEnrollment(_ context.Context, tenantID, nodeID string) (*core.NodeEnrollment, error) {
	record, ok := f.enrollments[tenantID+":"+nodeID]
	if !ok {
		return nil, nil
	}
	copy := record
	return &copy, nil
}

type fakeSessionLookup struct {
	boundaries  map[string]core.SessionBoundary
	delegations map[string][]core.SessionDelegationRecord
}

func (f fakeSessionLookup) GetBoundaryBySessionID(_ context.Context, sessionID string) (*core.SessionBoundary, error) {
	record, ok := f.boundaries[sessionID]
	if !ok {
		return nil, nil
	}
	copy := record
	return &copy, nil
}

func (f fakeSessionLookup) ListDelegationsBySessionID(_ context.Context, sessionID string) ([]core.SessionDelegationRecord, error) {
	records := f.delegations[sessionID]
	out := make([]core.SessionDelegationRecord, len(records))
	copy(out, records)
	return out, nil
}

type fakeTenantExportLookup struct {
	values map[string]bool
}

func (f fakeTenantExportLookup) IsExportEnabled(_ context.Context, tenantID, exportName string) (bool, bool, error) {
	value, ok := f.values[tenantID+":"+exportName]
	return value, ok, nil
}

type fakeTenantFederationLookup struct {
	policies map[string]core.TenantFederationPolicy
}

func (f fakeTenantFederationLookup) GetTenantFederationPolicy(_ context.Context, tenantID string) (*core.TenantFederationPolicy, error) {
	policy, ok := f.policies[tenantID]
	if !ok {
		return nil, nil
	}
	copy := policy
	return &copy, nil
}

func testRecipientKeys() StaticRecipientKeyResolver {
	return StaticRecipientKeyResolver{
		"runtime://mesh-a/node-1/rt-1": []byte("0123456789abcdef0123456789abcdef"),
		"runtime://mesh-a/node-2/rt-2": []byte("abcdef0123456789abcdef0123456789"),
		"runtime://mesh-a/node-b/rt-b": []byte("11111111111111112222222222222222"),
	}
}

func TestInMemoryOwnershipCommitAndFence(t *testing.T) {
	t.Parallel()

	store := &InMemoryOwnershipStore{}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	source := core.AttemptRecord{AttemptID: "attempt-a", LineageID: lineage.LineageID, RuntimeID: "rt-a", State: core.AttemptStateRunning, StartTime: time.Now().UTC()}
	dest := core.AttemptRecord{AttemptID: "attempt-b", LineageID: lineage.LineageID, RuntimeID: "rt-b", State: core.AttemptStateResumePending, StartTime: time.Now().UTC()}
	if err := store.UpsertAttempt(context.Background(), source); err != nil {
		t.Fatalf("UpsertAttempt(source) error = %v", err)
	}
	if err := store.UpsertAttempt(context.Background(), dest); err != nil {
		t.Fatalf("UpsertAttempt(dest) error = %v", err)
	}
	lease, err := store.IssueLease(context.Background(), lineage.LineageID, source.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease() error = %v", err)
	}
	if err := store.ValidateLease(context.Background(), *lease, time.Now().UTC()); err != nil {
		t.Fatalf("ValidateLease() error = %v", err)
	}
	commit := core.ResumeCommit{
		LineageID:            lineage.LineageID,
		OldAttemptID:         source.AttemptID,
		NewAttemptID:         dest.AttemptID,
		DestinationRuntimeID: dest.RuntimeID,
		ReceiptRef:           "receipt-1",
		CommitTime:           time.Now().UTC(),
	}
	if err := store.CommitHandoff(context.Background(), commit); err != nil {
		t.Fatalf("CommitHandoff() error = %v", err)
	}
	if err := store.Fence(context.Background(), core.FenceNotice{
		LineageID:    lineage.LineageID,
		AttemptID:    source.AttemptID,
		FencingEpoch: lease.FencingEpoch,
		Issuer:       "issuer",
	}); err != nil {
		t.Fatalf("Fence() error = %v", err)
	}
	fenced, ok, err := store.GetAttempt(context.Background(), source.AttemptID)
	if err != nil || !ok {
		t.Fatalf("GetAttempt() error = %v ok=%v", err, ok)
	}
	if !fenced.Fenced || fenced.State != core.AttemptStateFenced {
		t.Fatalf("fenced attempt = %+v", *fenced)
	}
}

func TestServiceOfferAcceptCommitLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &InMemoryOwnershipStore{}
	log := &memoryEventLog{}
	svc := &Service{
		Ownership: store,
		Packager: JSONPackager{
			RuntimeStore:      fakeWorkflowRuntimeStore{},
			KeyResolver:       testRecipientKeys(),
			DefaultRecipients: []string{"runtime://mesh-a/node-1/rt-1"},
			LocalRecipient:    "runtime://mesh-a/node-1/rt-1",
		},
		Nexus: NexusAdapter{Policies: allowResumePolicy{}},
		Log:   log,
		Now:   func() time.Time { return now },
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	if err := svc.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	source := core.AttemptRecord{AttemptID: "attempt-a", LineageID: lineage.LineageID, RuntimeID: "rt-a", State: core.AttemptStateRunning, StartTime: now}
	dest := core.AttemptRecord{AttemptID: "lineage-1:rt-b:resume", LineageID: lineage.LineageID, RuntimeID: "rt-b", State: core.AttemptStateResumePending, StartTime: now}
	if err := store.UpsertAttempt(context.Background(), source); err != nil {
		t.Fatalf("UpsertAttempt(source) error = %v", err)
	}
	if err := store.UpsertAttempt(context.Background(), dest); err != nil {
		t.Fatalf("UpsertAttempt(dest) error = %v", err)
	}
	offer, pkg, sealed, err := svc.OfferHandoff(context.Background(), lineage.LineageID, source.AttemptID, "exp.run", "issuer", RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("OfferHandoff() error = %v", err)
	}
	if pkg == nil || sealed == nil {
		t.Fatalf("OfferHandoff() returned nil package or sealed context")
	}
	accept, err := svc.AcceptHandoff(context.Background(), *offer, core.ExportDescriptor{
		ExportName:             "exp.run",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
	}, "rt-b")
	if err != nil {
		t.Fatalf("AcceptHandoff() error = %v", err)
	}
	receipt := core.ResumeReceipt{
		ReceiptID:         "receipt-1",
		LineageID:         lineage.LineageID,
		AttemptID:         accept.ProvisionalAttemptID,
		RuntimeID:         "rt-b",
		ImportedContextID: pkg.Manifest.ContextID,
		Status:            core.ReceiptStatusRunning,
	}
	if _, err := svc.CommitHandoff(context.Background(), *offer, *accept, receipt); err != nil {
		t.Fatalf("CommitHandoff() error = %v", err)
	}
	current, ok, err := store.GetLineage(context.Background(), lineage.LineageID)
	if err != nil || !ok {
		t.Fatalf("GetLineage() error = %v ok=%v", err, ok)
	}
	if current.CurrentOwnerRuntime != "rt-b" || current.CurrentOwnerAttempt != accept.ProvisionalAttemptID {
		t.Fatalf("lineage owner = %+v", *current)
	}
	if len(log.events) < 4 {
		t.Fatalf("expected audit events, got %d", len(log.events))
	}
}

func TestServiceResumeHandoffDrivesRuntimeLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &InMemoryOwnershipStore{}
	log := &memoryEventLog{}
	runtimeEndpoint := &fakeRuntimeEndpoint{
		descriptor: core.RuntimeDescriptor{
			RuntimeID:               "rt-b",
			NodeID:                  "node-b",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          1024,
		},
		createdAttempt: &core.AttemptRecord{
			AttemptID: "lineage-1:rt-b:resume",
			LineageID: "lineage-1",
			RuntimeID: "rt-b",
			State:     core.AttemptStateResumePending,
			StartTime: now,
		},
		issuedReceipt: &core.ResumeReceipt{
			ReceiptID:         "receipt-1",
			LineageID:         "lineage-1",
			AttemptID:         "lineage-1:rt-b:resume",
			RuntimeID:         "rt-b",
			ImportedContextID: "lineage-1:attempt-a",
			StartTime:         now,
			Status:            core.ReceiptStatusRunning,
		},
	}
	sourceSvc := &Service{
		Ownership: store,
		Packager: JSONPackager{
			RuntimeStore:      fakeWorkflowRuntimeStore{},
			KeyResolver:       testRecipientKeys(),
			DefaultRecipients: []string{"runtime://mesh-a/node-1/rt-1"},
			LocalRecipient:    "runtime://mesh-a/node-1/rt-1",
		},
		Nexus: NexusAdapter{Policies: allowResumePolicy{}},
		Log:   log,
		Now:   func() time.Time { return now },
	}
	destinationSvc := &Service{
		Ownership: store,
		Packager: JSONPackager{
			RuntimeStore:      fakeWorkflowRuntimeStore{},
			KeyResolver:       testRecipientKeys(),
			DefaultRecipients: []string{"runtime://mesh-a/node-1/rt-1"},
			LocalRecipient:    "runtime://mesh-a/node-1/rt-1",
		},
		Runtime: runtimeEndpoint,
		Nexus:   NexusAdapter{Policies: allowResumePolicy{}},
		Log:     log,
		Now:     func() time.Time { return now },
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	if err := sourceSvc.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	source := core.AttemptRecord{AttemptID: "attempt-a", LineageID: lineage.LineageID, RuntimeID: "rt-a", State: core.AttemptStateRunning, StartTime: now}
	if err := store.UpsertAttempt(context.Background(), source); err != nil {
		t.Fatalf("UpsertAttempt(source) error = %v", err)
	}
	offer, pkg, sealed, err := sourceSvc.OfferHandoff(context.Background(), lineage.LineageID, source.AttemptID, "exp.run", "issuer", RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("OfferHandoff() error = %v", err)
	}
	runtimeEndpoint.importPackage = pkg
	runtimeEndpoint.issuedReceipt.ImportedContextID = pkg.Manifest.ContextID
	executed, commit, refusal, err := destinationSvc.ResumeHandoff(context.Background(), *offer, core.ExportDescriptor{
		ExportName:             "exp.run",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
	}, "rt-b", pkg.Manifest, *sealed)
	if err != nil {
		t.Fatalf("ResumeHandoff() error = %v", err)
	}
	if refusal != nil {
		t.Fatalf("ResumeHandoff() refusal = %+v", refusal)
	}
	if executed == nil || commit == nil {
		t.Fatalf("ResumeHandoff() returned nil execution or commit")
	}
	if runtimeEndpoint.validateCalls != 1 || runtimeEndpoint.importCalls != 1 || runtimeEndpoint.createCalls != 1 || runtimeEndpoint.receiptCalls != 1 {
		t.Fatalf("runtime call counts = validate:%d import:%d create:%d receipt:%d", runtimeEndpoint.validateCalls, runtimeEndpoint.importCalls, runtimeEndpoint.createCalls, runtimeEndpoint.receiptCalls)
	}
	if len(runtimeEndpoint.fenceNotices) != 1 {
		t.Fatalf("fence notices = %+v", runtimeEndpoint.fenceNotices)
	}
	current, ok, err := store.GetLineage(context.Background(), lineage.LineageID)
	if err != nil || !ok {
		t.Fatalf("GetLineage() error = %v ok=%v", err, ok)
	}
	if current.CurrentOwnerAttempt != executed.Attempt.AttemptID || current.CurrentOwnerRuntime != "rt-b" {
		t.Fatalf("lineage owner = %+v", *current)
	}
}

func TestServiceResumeHandoffReleasesSlotOnRuntimeImportError(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &InMemoryOwnershipStore{}
	limiter := &InMemoryOperationalLimiter{Limits: OperationalLimits{MaxActiveResumeSlots: 1}}
	runtimeEndpoint := &fakeRuntimeEndpoint{
		descriptor: core.RuntimeDescriptor{
			RuntimeID:               "rt-b",
			NodeID:                  "node-b",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          1024,
		},
		importErr: fmt.Errorf("import failed"),
	}
	sourceSvc := &Service{
		Ownership: store,
		Packager: JSONPackager{
			RuntimeStore:      fakeWorkflowRuntimeStore{},
			KeyResolver:       testRecipientKeys(),
			DefaultRecipients: []string{"runtime://mesh-a/node-1/rt-1"},
			LocalRecipient:    "runtime://mesh-a/node-1/rt-1",
		},
		Nexus: NexusAdapter{Policies: allowResumePolicy{}},
	}
	destinationSvc := &Service{
		Ownership: store,
		Packager: JSONPackager{
			RuntimeStore:      fakeWorkflowRuntimeStore{},
			KeyResolver:       testRecipientKeys(),
			DefaultRecipients: []string{"runtime://mesh-a/node-1/rt-1"},
			LocalRecipient:    "runtime://mesh-a/node-1/rt-1",
		},
		Runtime: runtimeEndpoint,
		Nexus:   NexusAdapter{Policies: allowResumePolicy{}},
		Limiter: limiter,
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	source := core.AttemptRecord{AttemptID: "attempt-a", LineageID: lineage.LineageID, RuntimeID: "rt-a", State: core.AttemptStateRunning, StartTime: now}
	if err := store.UpsertAttempt(context.Background(), source); err != nil {
		t.Fatalf("UpsertAttempt() error = %v", err)
	}
	offer, pkg, sealed, err := sourceSvc.OfferHandoff(context.Background(), lineage.LineageID, source.AttemptID, "exp.run", "issuer", RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("OfferHandoff() error = %v", err)
	}
	if _, _, _, err := destinationSvc.ResumeHandoff(context.Background(), *offer, core.ExportDescriptor{
		ExportName:             "exp.run",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
	}, "rt-b", pkg.Manifest, *sealed); err == nil {
		t.Fatal("ResumeHandoff() error = nil, want runtime import failure")
	}
	snapshot, err := limiter.Snapshot(context.Background(), now)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.ActiveResumeSlots != 0 {
		t.Fatalf("active resume slots = %d, want 0", snapshot.ActiveResumeSlots)
	}
}

func TestResumeHandoffForNodeBindsSessionDelegationAndEnrollment(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &InMemoryOwnershipStore{}
	log := &memoryEventLog{}
	runtimeEndpoint := &fakeRuntimeEndpoint{
		descriptor: core.RuntimeDescriptor{
			RuntimeID:               "rt-b",
			NodeID:                  "node-b",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          1024,
		},
		createdAttempt: &core.AttemptRecord{
			AttemptID: "lineage-1:rt-b:resume",
			LineageID: "lineage-1",
			RuntimeID: "rt-b",
			State:     core.AttemptStateResumePending,
			StartTime: now,
		},
		issuedReceipt: &core.ResumeReceipt{
			ReceiptID:         "receipt-1",
			LineageID:         "lineage-1",
			AttemptID:         "lineage-1:rt-b:resume",
			RuntimeID:         "rt-b",
			Status:            core.ReceiptStatusRunning,
			ImportedContextID: "lineage-1:attempt-a",
			StartTime:         now,
		},
	}
	sourceSvc := &Service{
		Ownership: store,
		Packager: JSONPackager{
			RuntimeStore:      fakeWorkflowRuntimeStore{},
			KeyResolver:       testRecipientKeys(),
			DefaultRecipients: []string{"runtime://mesh-a/node-1/rt-1"},
			LocalRecipient:    "runtime://mesh-a/node-1/rt-1",
		},
		Nexus: NexusAdapter{Policies: allowResumePolicy{}},
		Log:   log,
		Now:   func() time.Time { return now },
	}
	destinationSvc := &Service{
		Ownership: store,
		Packager: JSONPackager{
			RuntimeStore:      fakeWorkflowRuntimeStore{},
			KeyResolver:       testRecipientKeys(),
			DefaultRecipients: []string{"runtime://mesh-a/node-1/rt-1"},
			LocalRecipient:    "runtime://mesh-a/node-1/rt-1",
		},
		Runtime: runtimeEndpoint,
		Nexus: NexusAdapter{
			Policies: allowResumePolicy{},
			Subjects: fakeSubjectLookup{subjects: map[string]core.SubjectRecord{
				"tenant-1:service_account:svc-1":      {TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1", CreatedAt: now},
				"tenant-1:service_account:delegate-1": {TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1", CreatedAt: now},
			}},
			Nodes: fakeNodeLookup{enrollments: map[string]core.NodeEnrollment{
				"tenant-1:node-b": {TenantID: "tenant-1", NodeID: "node-b", Owner: core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-b"}, TrustClass: core.TrustClassRemoteApproved, PairedAt: now, LastVerifiedAt: now},
			}},
			Sessions: fakeSessionLookup{
				boundaries: map[string]core.SessionBoundary{
					"sess-1": {SessionID: "sess-1", TenantID: "tenant-1", Owner: core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"}, TrustClass: core.TrustClassRemoteApproved, CreatedAt: now},
				},
				delegations: map[string][]core.SessionDelegationRecord{
					"sess-1": {{
						TenantID:   "tenant-1",
						SessionID:  "sess-1",
						Grantee:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1"},
						Operations: []core.SessionOperation{core.SessionOperationResume},
						CreatedAt:  now,
					}},
				},
			},
		},
		Log: log,
		Now: func() time.Time { return now },
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		SessionID:    "sess-1",
		TrustClass:   core.TrustClassRemoteApproved,
		Delegations: []core.SessionDelegationRecord{{
			TenantID:   "tenant-1",
			SessionID:  "sess-1",
			Grantee:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1"},
			Operations: []core.SessionOperation{core.SessionOperationResume},
			CreatedAt:  now,
		}},
	}
	if err := sourceSvc.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	source := core.AttemptRecord{AttemptID: "attempt-a", LineageID: lineage.LineageID, RuntimeID: "rt-a", State: core.AttemptStateRunning, StartTime: now}
	if err := store.UpsertAttempt(context.Background(), source); err != nil {
		t.Fatalf("UpsertAttempt(source) error = %v", err)
	}
	offer, pkg, sealed, err := sourceSvc.OfferHandoff(context.Background(), lineage.LineageID, source.AttemptID, "exp.run", "issuer", RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("OfferHandoff() error = %v", err)
	}
	runtimeEndpoint.importPackage = pkg
	runtimeEndpoint.issuedReceipt.ImportedContextID = pkg.Manifest.ContextID
	executed, commit, authorized, refusal, err := destinationSvc.ResumeHandoffForNode(context.Background(), *offer, core.ExportDescriptor{
		ExportName:             "exp.run",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
	}, "rt-b", "node-b", core.SubjectRef{
		TenantID: "tenant-1",
		Kind:     core.SubjectKindServiceAccount,
		ID:       "delegate-1",
	}, pkg.Manifest, *sealed)
	if err != nil {
		t.Fatalf("ResumeHandoffForNode() error = %v", err)
	}
	if refusal != nil {
		t.Fatalf("ResumeHandoffForNode() refusal = %+v", refusal)
	}
	if executed == nil || commit == nil || authorized == nil || !authorized.Delegated {
		t.Fatalf("ResumeHandoffForNode() = executed:%v commit:%v authorized:%+v", executed != nil, commit != nil, authorized)
	}
	foundBound := false
	for _, ev := range log.events {
		if ev.Type == core.FrameworkEventFMPContinuationBound {
			foundBound = true
			break
		}
	}
	if !foundBound {
		t.Fatalf("events = %+v, want continuation bound event", log.events)
	}
}

func TestResumeHandoffForNodeRejectsSessionOwnerDrift(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &InMemoryOwnershipStore{}
	runtimeEndpoint := &fakeRuntimeEndpoint{
		descriptor: core.RuntimeDescriptor{
			RuntimeID:               "rt-b",
			NodeID:                  "node-b",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          1024,
		},
	}
	sourceSvc := &Service{
		Ownership: store,
		Packager: JSONPackager{
			RuntimeStore:      fakeWorkflowRuntimeStore{},
			KeyResolver:       testRecipientKeys(),
			DefaultRecipients: []string{"runtime://mesh-a/node-1/rt-1"},
			LocalRecipient:    "runtime://mesh-a/node-1/rt-1",
		},
		Nexus: NexusAdapter{Policies: allowResumePolicy{}},
		Now:   func() time.Time { return now },
	}
	destinationSvc := &Service{
		Ownership: store,
		Runtime:   runtimeEndpoint,
		Nexus: NexusAdapter{
			Policies: allowResumePolicy{},
			Subjects: fakeSubjectLookup{subjects: map[string]core.SubjectRecord{
				"tenant-1:service_account:svc-1": {TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1", CreatedAt: now},
			}},
			Nodes: fakeNodeLookup{enrollments: map[string]core.NodeEnrollment{
				"tenant-1:node-b": {TenantID: "tenant-1", NodeID: "node-b", Owner: core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-b"}, TrustClass: core.TrustClassRemoteApproved, PairedAt: now, LastVerifiedAt: now},
			}},
			Sessions: fakeSessionLookup{
				boundaries: map[string]core.SessionBoundary{
					"sess-1": {SessionID: "sess-1", TenantID: "tenant-1", Owner: core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-2"}, TrustClass: core.TrustClassRemoteApproved, CreatedAt: now},
				},
			},
		},
		Now: func() time.Time { return now },
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		SessionID:    "sess-1",
		TrustClass:   core.TrustClassRemoteApproved,
	}
	if err := sourceSvc.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	source := core.AttemptRecord{AttemptID: "attempt-a", LineageID: lineage.LineageID, RuntimeID: "rt-a", State: core.AttemptStateRunning, StartTime: now}
	if err := store.UpsertAttempt(context.Background(), source); err != nil {
		t.Fatalf("UpsertAttempt() error = %v", err)
	}
	offer, pkg, sealed, err := sourceSvc.OfferHandoff(context.Background(), lineage.LineageID, source.AttemptID, "exp.run", "issuer", RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("OfferHandoff() error = %v", err)
	}
	_, _, _, _, err = destinationSvc.ResumeHandoffForNode(context.Background(), *offer, core.ExportDescriptor{
		ExportName:             "exp.run",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
	}, "rt-b", "node-b", lineage.Owner, pkg.Manifest, *sealed)
	if err == nil {
		t.Fatal("ResumeHandoffForNode() error = nil, want session owner drift failure")
	}
}

func TestServiceAcceptDeniedByPolicy(t *testing.T) {
	t.Parallel()

	store := &InMemoryOwnershipStore{}
	svc := &Service{
		Ownership: store,
		Packager: JSONPackager{
			RuntimeStore:      fakeWorkflowRuntimeStore{},
			KeyResolver:       testRecipientKeys(),
			DefaultRecipients: []string{"runtime://mesh-a/node-1/rt-1"},
			LocalRecipient:    "runtime://mesh-a/node-1/rt-1",
		},
		Nexus: NexusAdapter{Policies: denyResumePolicy{}},
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	attempt := core.AttemptRecord{AttemptID: "attempt-a", LineageID: lineage.LineageID, RuntimeID: "rt-a", State: core.AttemptStateRunning, StartTime: time.Now().UTC()}
	if err := store.UpsertAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("UpsertAttempt() error = %v", err)
	}
	lease, err := store.IssueLease(context.Background(), lineage.LineageID, attempt.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease() error = %v", err)
	}
	_, err = svc.AcceptHandoff(context.Background(), core.HandoffOffer{
		OfferID:            "offer-1",
		LineageID:          lineage.LineageID,
		SourceAttemptID:    attempt.AttemptID,
		SourceRuntimeID:    attempt.RuntimeID,
		DestinationExport:  "exp.run",
		ContextManifestRef: "ctx-1",
		ContextClass:       lineage.ContextClass,
		LeaseToken:         *lease,
		Expiry:             time.Now().UTC().Add(time.Minute),
	}, core.ExportDescriptor{ExportName: "exp.run"}, "rt-b")
	if err == nil {
		t.Fatal("AcceptHandoff() error = nil, want policy denial")
	}
}

func TestTryAcceptHandoffReturnsStructuredRefusalForContextSize(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &InMemoryOwnershipStore{}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	attempt := core.AttemptRecord{AttemptID: "attempt-a", LineageID: lineage.LineageID, RuntimeID: "rt-a", State: core.AttemptStateRunning, StartTime: now}
	if err := store.UpsertAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("UpsertAttempt() error = %v", err)
	}
	lease, err := store.IssueLease(context.Background(), lineage.LineageID, attempt.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease() error = %v", err)
	}
	svc := &Service{
		Ownership: store,
		Runtime: &fakeRuntimeEndpoint{descriptor: core.RuntimeDescriptor{
			RuntimeID:               "rt-b",
			NodeID:                  "node-1",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          64,
		}},
	}
	accept, refusal, err := svc.TryAcceptHandoff(context.Background(), core.HandoffOffer{
		OfferID:                  "offer-1",
		LineageID:                lineage.LineageID,
		SourceAttemptID:          attempt.AttemptID,
		SourceRuntimeID:          attempt.RuntimeID,
		SourceCompatibilityClass: "compat-a",
		DestinationExport:        "exp.run",
		ContextManifestRef:       "ctx-1",
		ContextClass:             lineage.ContextClass,
		ContextSizeBytes:         128,
		LeaseToken:               *lease,
		Expiry:                   now.Add(time.Minute),
	}, core.ExportDescriptor{
		ExportName:                   "exp.run",
		AcceptedContextClasses:       []string{"workflow-runtime"},
		RequiredCompatibilityClasses: []string{"compat-a"},
		MaxContextSize:               256,
	}, "rt-b")
	if err != nil {
		t.Fatalf("TryAcceptHandoff() error = %v", err)
	}
	if accept != nil || refusal == nil || refusal.Code != core.RefusalContextTooLarge {
		t.Fatalf("accept=%v refusal=%+v", accept, refusal)
	}
}

func TestTryAcceptHandoffRejectsDisallowedExportTransportPath(t *testing.T) {
	t.Parallel()

	store := &InMemoryOwnershipStore{}
	svc := &Service{Ownership: store}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-a",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-a",
		State:     core.AttemptStateRunning,
		StartTime: time.Now().UTC(),
	}
	if err := store.UpsertAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("UpsertAttempt() error = %v", err)
	}
	lease, err := store.IssueLease(context.Background(), lineage.LineageID, attempt.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease() error = %v", err)
	}
	offer := core.HandoffOffer{
		OfferID:            "offer-1",
		LineageID:          lineage.LineageID,
		SourceAttemptID:    attempt.AttemptID,
		SourceRuntimeID:    attempt.RuntimeID,
		DestinationExport:  "agent.resume",
		ContextManifestRef: "ctx-1",
		ContextClass:       lineage.ContextClass,
		ContextSizeBytes:   64,
		LeaseToken:         *lease,
		Expiry:             lease.Expiry,
	}
	destination := core.ExportDescriptor{
		ExportName:             "agent.resume",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
		AllowedTransportPaths:  []core.RouteMode{core.RouteModeDirect},
		AdmissionSummary:       core.AvailabilitySpec{Available: true},
	}
	accept, refusal, err := svc.TryAcceptHandoff(context.Background(), offer, destination, "rt-b")
	if err != nil {
		t.Fatalf("TryAcceptHandoff() error = %v", err)
	}
	if accept != nil || refusal == nil || refusal.Code != core.RefusalUnauthorized {
		t.Fatalf("accept=%+v refusal=%+v, want unauthorized refusal", accept, refusal)
	}
}

func TestTryAcceptHandoffRejectsBroadenedRequestedCapabilityProjection(t *testing.T) {
	t.Parallel()

	store := &InMemoryOwnershipStore{}
	svc := &Service{Ownership: store}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		CapabilityEnvelope: core.CapabilityEnvelope{
			AllowedCapabilityIDs: []string{"cap.read"},
			AllowedTaskClasses:   []string{"agent.run"},
			AllowOnwardExport:    false,
			MaxCPU:               1,
		},
	}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-a",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-a",
		State:     core.AttemptStateRunning,
		StartTime: time.Now().UTC(),
	}
	if err := store.UpsertAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("UpsertAttempt() error = %v", err)
	}
	lease, err := store.IssueLease(context.Background(), lineage.LineageID, attempt.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease() error = %v", err)
	}
	offer := core.HandoffOffer{
		OfferID:            "offer-1",
		LineageID:          lineage.LineageID,
		SourceAttemptID:    attempt.AttemptID,
		SourceRuntimeID:    attempt.RuntimeID,
		DestinationExport:  "agent.resume",
		ContextManifestRef: "ctx-1",
		ContextClass:       lineage.ContextClass,
		ContextSizeBytes:   64,
		RequestedCapabilityProjection: core.CapabilityEnvelope{
			AllowedCapabilityIDs: []string{"cap.read", "cap.write"},
			AllowedTaskClasses:   []string{"agent.run"},
			AllowOnwardExport:    true,
			MaxCPU:               2,
		},
		LeaseToken: *lease,
		Expiry:     lease.Expiry,
	}
	destination := core.ExportDescriptor{
		ExportName:             "agent.resume",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
		AdmissionSummary:       core.AvailabilitySpec{Available: true},
	}
	accept, refusal, err := svc.TryAcceptHandoff(context.Background(), offer, destination, "rt-b")
	if err != nil {
		t.Fatalf("TryAcceptHandoff() error = %v", err)
	}
	if accept != nil || refusal == nil || refusal.Code != core.RefusalUnauthorized {
		t.Fatalf("accept=%+v refusal=%+v, want unauthorized refusal", accept, refusal)
	}
}

func TestStrictCapabilityProjectorNarrowsCapabilities(t *testing.T) {
	t.Parallel()

	projected, err := StrictCapabilityProjector{}.Project(context.Background(), core.LineageRecord{
		CapabilityEnvelope: core.CapabilityEnvelope{
			AllowedCapabilityIDs: []string{"cap.a", "cap.b"},
			AllowedTaskClasses:   []string{"agent.run", "agent.debug"},
			AllowOnwardExport:    true,
		},
	}, core.ExportDescriptor{
		AllowedCapabilityIDs: []string{"cap.b"},
		AllowedTaskClasses:   []string{"agent.run"},
		AllowOnwardTransfer:  boolPtr(false),
	})
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if len(projected.AllowedCapabilityIDs) != 1 || projected.AllowedCapabilityIDs[0] != "cap.b" {
		t.Fatalf("projected capabilities = %+v", projected.AllowedCapabilityIDs)
	}
	if len(projected.AllowedTaskClasses) != 1 || projected.AllowedTaskClasses[0] != "agent.run" {
		t.Fatalf("projected task classes = %+v", projected.AllowedTaskClasses)
	}
	if projected.AllowOnwardExport {
		t.Fatalf("projected onward export should be false")
	}
}

func TestDiscoveryStoreDeletesExpiredAdvertisements(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &InMemoryDiscoveryStore{}
	if err := store.UpsertExportAdvertisement(context.Background(), core.ExportAdvertisement{
		TrustDomain: "local.mesh",
		Export: core.ExportDescriptor{
			ExportName: "exp.run",
		},
		ExpiresAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("UpsertExportAdvertisement() error = %v", err)
	}
	if err := store.DeleteExpired(context.Background(), now); err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}
	exports, err := store.ListExportAdvertisements(context.Background())
	if err != nil {
		t.Fatalf("ListExportAdvertisements() error = %v", err)
	}
	if len(exports) != 0 {
		t.Fatalf("exports = %+v, want empty", exports)
	}
}

func TestResolveRoutesPrefersLocalAndPreservesNamespaceIsolation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &InMemoryDiscoveryStore{}
	svc := &Service{
		Discovery: store,
		Now:       func() time.Time { return now },
	}
	requireAdvertiseRuntime := func(domain, runtimeID, compat string) {
		t.Helper()
		if err := svc.AdvertiseRuntime(context.Background(), core.RuntimeAdvertisement{
			TrustDomain: domain,
			Runtime: core.RuntimeDescriptor{
				RuntimeID:               runtimeID,
				NodeID:                  runtimeID + "-node",
				RuntimeVersion:          "1.0.0",
				CompatibilityClass:      compat,
				SupportedContextClasses: []string{"workflow-runtime"},
				MaxContextSize:          1024,
				AttestationProfile:      "test.attestation.v1",
				AttestationClaims:       map[string]string{"node_id": runtimeID + "-node"},
				Signature:               "sig-" + runtimeID,
			},
			Signature: "sig-" + runtimeID,
		}); err != nil {
			t.Fatalf("AdvertiseRuntime(%s) error = %v", runtimeID, err)
		}
	}
	requireAdvertiseRuntime("local.mesh", "rt-local", "compat-a")
	requireAdvertiseRuntime("remote.mesh", "rt-remote", "compat-a")
	if err := svc.AdvertiseExport(context.Background(), core.ExportAdvertisement{
		TrustDomain: "local.mesh",
		RuntimeID:   "rt-local",
		NodeID:      "rt-local-node",
		Export: core.ExportDescriptor{
			ExportName:             "exp.run",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeDirect,
			AdmissionSummary:       core.AvailabilitySpec{Available: true},
		},
	}); err != nil {
		t.Fatalf("AdvertiseExport(local) error = %v", err)
	}
	if err := svc.AdvertiseExport(context.Background(), core.ExportAdvertisement{
		TrustDomain: "remote.mesh",
		RuntimeID:   "rt-remote",
		NodeID:      "rt-remote-node",
		Imported:    true,
		Export: core.ExportDescriptor{
			ExportName:             "exp.run",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeGateway,
			AdmissionSummary:       core.AvailabilitySpec{Available: true},
		},
	}); err != nil {
		t.Fatalf("AdvertiseExport(remote) error = %v", err)
	}

	routes, err := svc.ResolveRoutes(context.Background(), RouteSelectionRequest{
		ExportName:       "exp.run",
		ContextClass:     "workflow-runtime",
		ContextSizeBytes: 64,
	})
	if err != nil {
		t.Fatalf("ResolveRoutes() error = %v", err)
	}
	if len(routes) != 1 || routes[0].Imported {
		t.Fatalf("routes = %+v, want only local route", routes)
	}

	qualifiedRoutes, err := svc.ResolveRoutes(context.Background(), RouteSelectionRequest{
		ExportName:       QualifiedExportName("remote.mesh", "exp.run"),
		ContextClass:     "workflow-runtime",
		ContextSizeBytes: 64,
		AllowRemote:      true,
	})
	if err != nil {
		t.Fatalf("ResolveRoutes(qualified) error = %v", err)
	}
	if len(qualifiedRoutes) != 1 || !qualifiedRoutes[0].Imported || qualifiedRoutes[0].TrustDomain != "remote.mesh" {
		t.Fatalf("qualified routes = %+v", qualifiedRoutes)
	}
}

func TestResolveRoutesFiltersByCompatibilityAndAdmission(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &InMemoryDiscoveryStore{}
	svc := &Service{
		Discovery: store,
		Now:       func() time.Time { return now },
	}
	if err := svc.AdvertiseRuntime(context.Background(), core.RuntimeAdvertisement{
		TrustDomain: "local.mesh",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:               "rt-local",
			NodeID:                  "node-local",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-b",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          1024,
			AttestationProfile:      "test.attestation.v1",
			AttestationClaims:       map[string]string{"node_id": "node-local"},
			Signature:               "sig-rt-local",
		},
		Signature: "sig-rt-local",
	}); err != nil {
		t.Fatalf("AdvertiseRuntime() error = %v", err)
	}
	if err := svc.AdvertiseExport(context.Background(), core.ExportAdvertisement{
		TrustDomain: "local.mesh",
		RuntimeID:   "rt-local",
		NodeID:      "node-local",
		Export: core.ExportDescriptor{
			ExportName:                   "exp.run",
			AcceptedContextClasses:       []string{"workflow-runtime"},
			RequiredCompatibilityClasses: []string{"compat-a"},
			AdmissionSummary:             core.AvailabilitySpec{Available: false, Reason: "busy"},
		},
	}); err != nil {
		t.Fatalf("AdvertiseExport() error = %v", err)
	}

	routes, err := svc.ResolveRoutes(context.Background(), RouteSelectionRequest{
		ExportName:                 "exp.run",
		ContextClass:               "workflow-runtime",
		ContextSizeBytes:           64,
		RequiredCompatibilityClass: "compat-a",
	})
	if err != nil {
		t.Fatalf("ResolveRoutes() error = %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("routes = %+v, want none", routes)
	}
}

func TestRegisterRuntimeRequiresAttestationAndPublishesNodeAndRuntime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &InMemoryDiscoveryStore{}
	svc := &Service{
		Discovery: store,
		Now:       func() time.Time { return now },
	}
	req := RuntimeRegistrationRequest{
		TrustDomain: "mesh.local",
		Node: core.NodeDescriptor{
			ID:         "node-1",
			TenantID:   "tenant-1",
			Name:       "Node One",
			Platform:   core.NodePlatformHeadless,
			TrustClass: core.TrustClassRemoteApproved,
			Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
			PairedAt:   now.Add(-time.Hour),
		},
		Runtime: core.RuntimeDescriptor{
			RuntimeID:               "runtime-1",
			NodeID:                  "node-1",
			TrustDomain:             "mesh.local",
			RuntimeVersion:          "1.2.3",
			SupportedContextClasses: []string{"workflow-runtime"},
			CompatibilityClass:      "compat-a",
			AttestationProfile:      "nexus.node_enrollment.v1",
			AttestationClaims: map[string]string{
				"peer_key_id": "key-1",
				"transport":   "websocket.tls.v1",
			},
			Signature: "sig-1",
		},
		ExpiresAt: now.Add(time.Minute),
	}
	if err := svc.RegisterRuntime(context.Background(), req); err != nil {
		t.Fatalf("RegisterRuntime() error = %v", err)
	}
	runtimes, err := store.ListRuntimeAdvertisements(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimeAdvertisements() error = %v", err)
	}
	if len(runtimes) != 1 {
		t.Fatalf("runtime advertisement count = %d, want 1", len(runtimes))
	}
	if runtimes[0].Runtime.AttestationProfile != "nexus.node_enrollment.v1" || runtimes[0].Runtime.Signature != "sig-1" {
		t.Fatalf("unexpected runtime advertisement = %+v", runtimes[0])
	}
	nodes, err := store.ListNodeAdvertisements(context.Background())
	if err != nil {
		t.Fatalf("ListNodeAdvertisements() error = %v", err)
	}
	if len(nodes) != 1 || nodes[0].Node.ID != "node-1" {
		t.Fatalf("unexpected node advertisements = %+v", nodes)
	}
}

func TestRegisterRuntimeRejectsMissingAttestation(t *testing.T) {
	t.Parallel()

	svc := &Service{Discovery: &InMemoryDiscoveryStore{}}
	err := svc.RegisterRuntime(context.Background(), RuntimeRegistrationRequest{
		TrustDomain: "mesh.local",
		Node: core.NodeDescriptor{
			ID:         "node-1",
			TenantID:   "tenant-1",
			Name:       "Node One",
			Platform:   core.NodePlatformHeadless,
			TrustClass: core.TrustClassRemoteApproved,
			Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
			PairedAt:   time.Now().UTC(),
		},
		Runtime: core.RuntimeDescriptor{
			RuntimeID:      "runtime-1",
			NodeID:         "node-1",
			RuntimeVersion: "1.2.3",
		},
	})
	if err == nil {
		t.Fatal("RegisterRuntime() error = nil, want attestation failure")
	}
}

func TestAdvertiseExportRejectsUnregisteredRuntime(t *testing.T) {
	t.Parallel()

	svc := &Service{Discovery: &InMemoryDiscoveryStore{}}
	err := svc.AdvertiseExport(context.Background(), core.ExportAdvertisement{
		TrustDomain: "mesh.local",
		RuntimeID:   "rt-missing",
		NodeID:      "node-missing",
		Export: core.ExportDescriptor{
			ExportName:             "exp.run",
			AcceptedContextClasses: []string{"workflow-runtime"},
		},
	})
	if err == nil {
		t.Fatal("AdvertiseExport() error = nil, want unregistered runtime failure")
	}
}

func TestResolveRoutesFiltersByRequestedRouteMode(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &InMemoryDiscoveryStore{}
	svc := &Service{
		Discovery: store,
		Now:       func() time.Time { return now },
	}
	if err := svc.AdvertiseRuntime(context.Background(), core.RuntimeAdvertisement{
		TrustDomain: "local.mesh",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:               "rt-local",
			NodeID:                  "node-local",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          1024,
			AttestationProfile:      "test.attestation.v1",
			AttestationClaims:       map[string]string{"node_id": "node-local"},
			Signature:               "sig-rt-local",
		},
		Signature: "sig-rt-local",
	}); err != nil {
		t.Fatalf("AdvertiseRuntime() error = %v", err)
	}
	if err := svc.AdvertiseExport(context.Background(), core.ExportAdvertisement{
		TrustDomain: "local.mesh",
		RuntimeID:   "rt-local",
		NodeID:      "node-local",
		Export: core.ExportDescriptor{
			ExportName:             "exp.run",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeGateway,
			AllowedTransportPaths:  []core.RouteMode{core.RouteModeGateway},
			AdmissionSummary:       core.AvailabilitySpec{Available: true},
		},
	}); err != nil {
		t.Fatalf("AdvertiseExport() error = %v", err)
	}
	routes, err := svc.ResolveRoutes(context.Background(), RouteSelectionRequest{
		ExportName:        "exp.run",
		ContextClass:      "workflow-runtime",
		ContextSizeBytes:  64,
		RequiredRouteMode: core.RouteModeDirect,
	})
	if err != nil {
		t.Fatalf("ResolveRoutes() error = %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("routes = %+v, want none for direct-only request", routes)
	}
}

func TestResolveRoutesFiltersByTaskClass(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &InMemoryDiscoveryStore{}
	svc := &Service{
		Discovery: store,
		Now:       func() time.Time { return now },
	}
	if err := svc.AdvertiseRuntime(context.Background(), core.RuntimeAdvertisement{
		TrustDomain: "local.mesh",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:               "rt-local",
			NodeID:                  "node-local",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          1024,
			AttestationProfile:      "test.attestation.v1",
			AttestationClaims:       map[string]string{"node_id": "node-local"},
			Signature:               "sig-rt-local",
		},
		Signature: "sig-rt-local",
	}); err != nil {
		t.Fatalf("AdvertiseRuntime() error = %v", err)
	}
	if err := svc.AdvertiseExport(context.Background(), core.ExportAdvertisement{
		TrustDomain: "local.mesh",
		RuntimeID:   "rt-local",
		NodeID:      "node-local",
		Export: core.ExportDescriptor{
			ExportName:             "exp.run",
			AcceptedContextClasses: []string{"workflow-runtime"},
			AllowedTaskClasses:     []string{"agent.run"},
			RouteMode:              core.RouteModeGateway,
			AdmissionSummary:       core.AvailabilitySpec{Available: true},
		},
	}); err != nil {
		t.Fatalf("AdvertiseExport() error = %v", err)
	}
	routes, err := svc.ResolveRoutes(context.Background(), RouteSelectionRequest{
		ExportName:       "exp.run",
		TaskClass:        "agent.debug",
		ContextClass:     "workflow-runtime",
		ContextSizeBytes: 64,
	})
	if err != nil {
		t.Fatalf("ResolveRoutes() error = %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("routes = %+v, want none for disallowed task class", routes)
	}
}

func TestResolveRoutesFiltersByAcceptedIdentityUsingLineageOwner(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	discovery := &InMemoryDiscoveryStore{}
	ownership := &InMemoryOwnershipStore{}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "owner-1"},
	}
	if err := ownership.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	svc := &Service{
		Discovery: discovery,
		Ownership: ownership,
		Now:       func() time.Time { return now },
	}
	if err := svc.AdvertiseRuntime(context.Background(), core.RuntimeAdvertisement{
		TrustDomain: "local.mesh",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:               "rt-local",
			NodeID:                  "node-local",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          1024,
			AttestationProfile:      "test.attestation.v1",
			AttestationClaims:       map[string]string{"node_id": "node-local"},
			Signature:               "sig-rt-local",
		},
		Signature: "sig-rt-local",
	}); err != nil {
		t.Fatalf("AdvertiseRuntime() error = %v", err)
	}
	if err := svc.AdvertiseExport(context.Background(), core.ExportAdvertisement{
		TrustDomain: "local.mesh",
		RuntimeID:   "rt-local",
		NodeID:      "node-local",
		Export: core.ExportDescriptor{
			ExportName:             "exp.run",
			AcceptedContextClasses: []string{"workflow-runtime"},
			AcceptedIdentities: []core.SubjectRef{{
				TenantID: "tenant-1",
				Kind:     core.SubjectKindServiceAccount,
				ID:       "other-owner",
			}},
			RouteMode:        core.RouteModeGateway,
			AdmissionSummary: core.AvailabilitySpec{Available: true},
		},
	}); err != nil {
		t.Fatalf("AdvertiseExport() error = %v", err)
	}
	routes, err := svc.ResolveRoutes(context.Background(), RouteSelectionRequest{
		LineageID:        lineage.LineageID,
		ExportName:       "exp.run",
		TaskClass:        lineage.TaskClass,
		ContextClass:     lineage.ContextClass,
		ContextSizeBytes: 64,
	})
	if err != nil {
		t.Fatalf("ResolveRoutes() error = %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("routes = %+v, want none for unauthorized owner", routes)
	}
}

func TestResolveRoutesFiltersByResumePolicyActorContext(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	discovery := &InMemoryDiscoveryStore{}
	svc := &Service{
		Discovery: discovery,
		Nexus:     NexusAdapter{Policies: delegatedOnlyResumePolicy{}},
		Now:       func() time.Time { return now },
	}
	if err := svc.AdvertiseRuntime(context.Background(), core.RuntimeAdvertisement{
		TrustDomain: "local.mesh",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:               "rt-local",
			NodeID:                  "node-local",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          1024,
			AttestationProfile:      "test.attestation.v1",
			AttestationClaims:       map[string]string{"node_id": "node-local"},
			Signature:               "sig-rt-local",
		},
		Signature: "sig-rt-local",
	}); err != nil {
		t.Fatalf("AdvertiseRuntime() error = %v", err)
	}
	if err := svc.AdvertiseExport(context.Background(), core.ExportAdvertisement{
		TrustDomain: "local.mesh",
		RuntimeID:   "rt-local",
		NodeID:      "node-local",
		Export: core.ExportDescriptor{
			ExportName:             "exp.run",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeGateway,
			AdmissionSummary:       core.AvailabilitySpec{Available: true},
		},
	}); err != nil {
		t.Fatalf("AdvertiseExport() error = %v", err)
	}
	baseReq := RouteSelectionRequest{
		TenantID:         "tenant-1",
		Owner:            core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "owner-1"},
		Actor:            core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "worker-1"},
		IsOwner:          false,
		IsDelegated:      false,
		TaskClass:        "agent.run",
		ContextClass:     "workflow-runtime",
		ContextSizeBytes: 64,
		SensitivityClass: core.SensitivityClassModerate,
		TrustClass:       core.TrustClassRemoteApproved,
		SessionID:        "sess-1",
		ExportName:       "exp.run",
	}
	routes, err := svc.ResolveRoutes(context.Background(), baseReq)
	if err != nil {
		t.Fatalf("ResolveRoutes() error = %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("routes = %+v, want none without delegation", routes)
	}
	baseReq.IsDelegated = true
	routes, err = svc.ResolveRoutes(context.Background(), baseReq)
	if err != nil {
		t.Fatalf("ResolveRoutes(delegated) error = %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("routes = %+v, want one delegated route", routes)
	}
}

func TestResolveRoutesFiltersDisabledTenantExport(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	discovery := &InMemoryDiscoveryStore{}
	svc := &Service{
		Discovery: discovery,
		Nexus: NexusAdapter{
			Exports: fakeTenantExportLookup{values: map[string]bool{
				"tenant-1:exp.run": false,
			}},
		},
		Now: func() time.Time { return now },
	}
	if err := svc.AdvertiseRuntime(context.Background(), core.RuntimeAdvertisement{
		TrustDomain: "local.mesh",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:               "rt-local",
			NodeID:                  "node-local",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          1024,
			AttestationProfile:      "test.attestation.v1",
			AttestationClaims:       map[string]string{"node_id": "node-local"},
			Signature:               "sig-rt-local",
		},
		Signature: "sig-rt-local",
	}); err != nil {
		t.Fatalf("AdvertiseRuntime() error = %v", err)
	}
	if err := svc.AdvertiseExport(context.Background(), core.ExportAdvertisement{
		TrustDomain: "local.mesh",
		RuntimeID:   "rt-local",
		NodeID:      "node-local",
		Export: core.ExportDescriptor{
			ExportName:             "exp.run",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeGateway,
			AdmissionSummary:       core.AvailabilitySpec{Available: true},
		},
	}); err != nil {
		t.Fatalf("AdvertiseExport() error = %v", err)
	}
	routes, err := svc.ResolveRoutes(context.Background(), RouteSelectionRequest{
		TenantID:         "tenant-1",
		ExportName:       "exp.run",
		TaskClass:        "agent.run",
		ContextClass:     "workflow-runtime",
		ContextSizeBytes: 64,
	})
	if err != nil {
		t.Fatalf("ResolveRoutes() error = %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("routes = %+v, want none for tenant-disabled export", routes)
	}
}

func TestTryAcceptHandoffRejectsDisabledTenantExport(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &InMemoryOwnershipStore{}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-a",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-a",
		State:     core.AttemptStateRunning,
		StartTime: now,
	}
	if err := store.UpsertAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("UpsertAttempt() error = %v", err)
	}
	lease, err := store.IssueLease(context.Background(), lineage.LineageID, attempt.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease() error = %v", err)
	}
	svc := &Service{
		Ownership: store,
		Nexus: NexusAdapter{
			Exports: fakeTenantExportLookup{values: map[string]bool{
				"tenant-1:exp.run": false,
			}},
		},
	}
	_, refusal, err := svc.TryAcceptHandoff(context.Background(), core.HandoffOffer{
		OfferID:            "offer-1",
		LineageID:          lineage.LineageID,
		SourceAttemptID:    attempt.AttemptID,
		SourceRuntimeID:    attempt.RuntimeID,
		DestinationExport:  "exp.run",
		ContextManifestRef: "ctx-1",
		ContextClass:       lineage.ContextClass,
		ContextSizeBytes:   64,
		LeaseToken:         *lease,
		Expiry:             lease.Expiry,
	}, core.ExportDescriptor{
		ExportName:             "exp.run",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
		AdmissionSummary:       core.AvailabilitySpec{Available: true},
	}, "rt-b")
	if err != nil {
		t.Fatalf("TryAcceptHandoff() error = %v", err)
	}
	if refusal == nil || refusal.Code != core.RefusalAdmissionClosed {
		t.Fatalf("refusal = %+v, want admission closed for tenant-disabled export", refusal)
	}
}

func TestResolveRoutesRejectsRemoteTrustDomainNotAllowedForTenant(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	svc := &Service{
		Discovery:  &InMemoryDiscoveryStore{},
		Trust:      &InMemoryTrustBundleStore{},
		Boundaries: &InMemoryBoundaryPolicyStore{},
		Nexus: NexusAdapter{
			Federation: fakeTenantFederationLookup{policies: map[string]core.TenantFederationPolicy{
				"tenant-1": {TenantID: "tenant-1", AllowedTrustDomains: []string{"mesh.allowed"}},
			}},
		},
		Now: func() time.Time { return now },
	}
	gateway := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
	if err := svc.RegisterTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain:       "mesh.remote",
		BundleID:          "bundle-1",
		GatewayIdentities: []core.SubjectRef{gateway},
		ExpiresAt:         now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("RegisterTrustBundle() error = %v", err)
	}
	if err := svc.SetBoundaryPolicy(context.Background(), core.BoundaryPolicy{
		TrustDomain:                  "mesh.remote",
		AcceptedSourceDomains:        []string{"mesh.remote"},
		AcceptedSourceIdentities:     []core.SubjectRef{gateway},
		AllowedRouteModes:            []core.RouteMode{core.RouteModeGateway},
		RequireGatewayAuthentication: true,
		MaxTransferBytes:             4096,
	}); err != nil {
		t.Fatalf("SetBoundaryPolicy() error = %v", err)
	}
	if err := svc.ImportFederatedRuntimeAdvertisement(context.Background(), gateway, core.RuntimeAdvertisement{
		TrustDomain: "mesh.remote",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:               "rt-remote",
			NodeID:                  "node-remote",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          2048,
			AttestationProfile:      "remote.runtime.v1",
			AttestationClaims:       map[string]string{"node_id": "node-remote"},
			Signature:               "sig-remote",
		},
		Signature: "sig-remote",
	}, "mesh.remote"); err != nil {
		t.Fatalf("ImportFederatedRuntimeAdvertisement() error = %v", err)
	}
	if err := svc.ImportFederatedExportAdvertisement(context.Background(), gateway, core.ExportAdvertisement{
		TrustDomain: "mesh.remote",
		Export: core.ExportDescriptor{
			ExportName:             "agent.resume",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeGateway,
			MaxContextSize:         2048,
		},
		RuntimeID: "rt-remote",
		NodeID:    "node-remote",
	}, "mesh.remote"); err != nil {
		t.Fatalf("ImportFederatedExportAdvertisement() error = %v", err)
	}
	routes, err := svc.ResolveRoutes(context.Background(), RouteSelectionRequest{
		TenantID:         "tenant-1",
		ExportName:       QualifiedExportName("mesh.remote", "agent.resume"),
		ContextClass:     "workflow-runtime",
		ContextSizeBytes: 512,
		AllowRemote:      true,
	})
	if err != nil {
		t.Fatalf("ResolveRoutes() error = %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("routes = %+v, want none for tenant-disallowed trust domain", routes)
	}
}

func TestCreateLineageFromSessionLoadsDelegations(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &InMemoryOwnershipStore{}
	svc := &Service{
		Ownership: store,
		Nexus: NexusAdapter{
			Tenants: fakeTenantLookup{tenants: map[string]core.TenantRecord{
				"tenant-1": {ID: "tenant-1", CreatedAt: now},
			}},
			Subjects: fakeSubjectLookup{subjects: map[string]core.SubjectRecord{
				"tenant-1:service_account:svc-1":      {TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1", CreatedAt: now},
				"tenant-1:service_account:delegate-1": {TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1", CreatedAt: now},
			}},
			Sessions: fakeSessionLookup{
				boundaries: map[string]core.SessionBoundary{
					"sess-1": {
						SessionID:  "sess-1",
						TenantID:   "tenant-1",
						Partition:  "local",
						Scope:      core.SessionScopePerChannelPeer,
						Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
						TrustClass: core.TrustClassRemoteApproved,
					},
				},
				delegations: map[string][]core.SessionDelegationRecord{
					"sess-1": {{
						TenantID:   "tenant-1",
						SessionID:  "sess-1",
						Grantee:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1"},
						Operations: []core.SessionOperation{core.SessionOperationResume},
						CreatedAt:  now,
					}},
				},
			},
		},
		Now: func() time.Time { return now },
	}

	lineage, err := svc.CreateLineageFromSession(context.Background(), SessionLineageRequest{
		LineageID:    "lineage-1",
		SessionID:    "sess-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
	})
	if err != nil {
		t.Fatalf("CreateLineageFromSession() error = %v", err)
	}
	if lineage.TenantID != "tenant-1" || lineage.SessionID != "sess-1" {
		t.Fatalf("lineage = %+v", *lineage)
	}
	if len(lineage.Delegations) != 1 {
		t.Fatalf("delegations = %+v", lineage.Delegations)
	}
}

func TestAuthorizeResumeActorAllowsDelegation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	store := &InMemoryOwnershipStore{}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		SessionID:    "sess-1",
		Delegations: []core.SessionDelegationRecord{{
			TenantID:   "tenant-1",
			SessionID:  "sess-1",
			Grantee:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1"},
			Operations: []core.SessionOperation{core.SessionOperationResume},
			CreatedAt:  now,
		}},
	}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	svc := &Service{
		Ownership: store,
		Nexus: NexusAdapter{
			Subjects: fakeSubjectLookup{subjects: map[string]core.SubjectRecord{
				"tenant-1:service_account:delegate-1": {TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1", CreatedAt: now},
			}},
		},
		Now: func() time.Time { return now },
	}
	authz, err := svc.AuthorizeResumeActor(context.Background(), lineage.LineageID, core.SubjectRef{
		TenantID: "tenant-1",
		Kind:     core.SubjectKindServiceAccount,
		ID:       "delegate-1",
	}, core.SessionOperationResume)
	if err != nil {
		t.Fatalf("AuthorizeResumeActor() error = %v", err)
	}
	if !authz.Delegated {
		t.Fatalf("authz = %+v", *authz)
	}
}

func TestAcceptHandoffForNodeRejectsCrossTenantActor(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &InMemoryOwnershipStore{}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		SessionID:    "sess-1",
	}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	source := core.AttemptRecord{AttemptID: "attempt-a", LineageID: lineage.LineageID, RuntimeID: "rt-a", State: core.AttemptStateRunning, StartTime: now}
	if err := store.UpsertAttempt(context.Background(), source); err != nil {
		t.Fatalf("UpsertAttempt() error = %v", err)
	}
	lease, err := store.IssueLease(context.Background(), lineage.LineageID, source.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease() error = %v", err)
	}
	svc := &Service{
		Ownership: store,
		Nexus: NexusAdapter{
			Nodes: fakeNodeLookup{enrollments: map[string]core.NodeEnrollment{
				"tenant-1:node-1": {
					TenantID:   "tenant-1",
					NodeID:     "node-1",
					Owner:      lineage.Owner,
					TrustClass: core.TrustClassRemoteApproved,
					PairedAt:   now,
				},
			}},
		},
	}
	_, _, err = svc.AcceptHandoffForNode(context.Background(), core.HandoffOffer{
		OfferID:            "offer-1",
		LineageID:          lineage.LineageID,
		SourceAttemptID:    source.AttemptID,
		SourceRuntimeID:    source.RuntimeID,
		DestinationExport:  "exp.run",
		ContextManifestRef: "ctx-1",
		ContextClass:       lineage.ContextClass,
		LeaseToken:         *lease,
		Expiry:             now.Add(time.Minute),
	}, core.ExportDescriptor{ExportName: "exp.run"}, "rt-b", "node-1", core.SubjectRef{
		TenantID: "tenant-2",
		Kind:     core.SubjectKindServiceAccount,
		ID:       "svc-x",
	})
	if err == nil {
		t.Fatal("AcceptHandoffForNode() error = nil, want cross-tenant denial")
	}
}

func TestAcceptHandoffForNodeRequiresEnrollment(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &InMemoryOwnershipStore{}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		SessionID:    "sess-1",
	}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	source := core.AttemptRecord{AttemptID: "attempt-a", LineageID: lineage.LineageID, RuntimeID: "rt-a", State: core.AttemptStateRunning, StartTime: now}
	if err := store.UpsertAttempt(context.Background(), source); err != nil {
		t.Fatalf("UpsertAttempt() error = %v", err)
	}
	lease, err := store.IssueLease(context.Background(), lineage.LineageID, source.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease() error = %v", err)
	}
	svc := &Service{
		Ownership: store,
		Nexus: NexusAdapter{
			Nodes: fakeNodeLookup{enrollments: map[string]core.NodeEnrollment{}},
		},
	}
	_, _, err = svc.AcceptHandoffForNode(context.Background(), core.HandoffOffer{
		OfferID:            "offer-1",
		LineageID:          lineage.LineageID,
		SourceAttemptID:    source.AttemptID,
		SourceRuntimeID:    source.RuntimeID,
		DestinationExport:  "exp.run",
		ContextManifestRef: "ctx-1",
		ContextClass:       lineage.ContextClass,
		LeaseToken:         *lease,
		Expiry:             now.Add(time.Minute),
	}, core.ExportDescriptor{ExportName: "exp.run"}, "rt-b", "node-missing", lineage.Owner)
	if err == nil {
		t.Fatal("AcceptHandoffForNode() error = nil, want enrollment failure")
	}
}

type fakeWorkflowRuntimeStore struct{}

func (fakeWorkflowRuntimeStore) QueryWorkflowRuntime(context.Context, string, string) (map[string]any, error) {
	return map[string]any{
		"workflow_id": "wf-1",
		"run_id":      "run-1",
		"events":      []string{"checkpointed"},
	}, nil
}

func boolPtr(v bool) *bool { return &v }
