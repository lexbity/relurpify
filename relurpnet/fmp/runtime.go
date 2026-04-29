package fmp

import (
	"context"
	"strings"
)

type PortableContextPackage struct {
	Manifest             ContextManifest `json:"manifest" yaml:"manifest"`
	ExecutionPayload     []byte               `json:"execution_payload,omitempty" yaml:"execution_payload,omitempty"`
	DeclarativeMemory    []byte               `json:"declarative_memory,omitempty" yaml:"declarative_memory,omitempty"`
	ProceduralMemory     []byte               `json:"procedural_memory,omitempty" yaml:"procedural_memory,omitempty"`
	RetrievalReferences  []string             `json:"retrieval_references,omitempty" yaml:"retrieval_references,omitempty"`
	AdditionalObjectRefs []string             `json:"additional_object_refs,omitempty" yaml:"additional_object_refs,omitempty"`
}

type ImportResult struct {
	Attempt AttemptRecord `json:"attempt" yaml:"attempt"`
	Receipt ResumeReceipt `json:"receipt" yaml:"receipt"`
}

// RuntimeEndpoint is part of the Phase 1 frozen FMP surface.
// The current implementation only partially exercises it; later phases should
// drive real runtime validation/import/receipt behavior through this interface.
type RuntimeEndpoint interface {
	Descriptor(ctx context.Context) (RuntimeDescriptor, error)
	ExportContext(ctx context.Context, lineage LineageRecord, attempt AttemptRecord) (*PortableContextPackage, error)
	ValidateContext(ctx context.Context, manifest ContextManifest, sealed SealedContext) error
	ImportContext(ctx context.Context, lineage LineageRecord, manifest ContextManifest, sealed SealedContext) (*PortableContextPackage, error)
	CreateAttempt(ctx context.Context, lineage LineageRecord, accept HandoffAccept, pkg *PortableContextPackage) (*AttemptRecord, error)
	FenceAttempt(ctx context.Context, notice FenceNotice) error
	IssueReceipt(ctx context.Context, lineage LineageRecord, attempt AttemptRecord, pkg *PortableContextPackage) (*ResumeReceipt, error)
}

// CapabilityProjector is the policy projection seam preserved across the
// closure phases so stricter policy logic can replace the current narrowing
// behavior without service churn.
type CapabilityProjector interface {
	Project(ctx context.Context, lineage LineageRecord, destination ExportDescriptor) (CapabilityEnvelope, error)
}

type StrictCapabilityProjector struct{}

func (StrictCapabilityProjector) Project(_ context.Context, lineage LineageRecord, destination ExportDescriptor) (CapabilityEnvelope, error) {
	projected := lineage.CapabilityEnvelope
	if len(destination.AllowedCapabilityIDs) > 0 {
		projected.AllowedCapabilityIDs = intersectFold(projected.AllowedCapabilityIDs, destination.AllowedCapabilityIDs)
	}
	if len(destination.AllowedTaskClasses) > 0 {
		projected.AllowedTaskClasses = intersectFold(projected.AllowedTaskClasses, destination.AllowedTaskClasses)
	}
	if destination.AllowOnwardTransfer != nil {
		projected.AllowOnwardExport = projected.AllowOnwardExport && *destination.AllowOnwardTransfer
	}
	if err := projected.Validate(); err != nil {
		return CapabilityEnvelope{}, err
	}
	return projected, nil
}

func capabilityEnvelopeSubset(requested, baseline CapabilityEnvelope) bool {
	if !subsetFold(requested.AllowedCapabilityIDs, baseline.AllowedCapabilityIDs) {
		return false
	}
	if !subsetFold(requested.AllowedTaskClasses, baseline.AllowedTaskClasses) {
		return false
	}
	if baseline.AllowChildTasks == false && requested.AllowChildTasks {
		return false
	}
	if baseline.AllowOnwardExport == false && requested.AllowOnwardExport {
		return false
	}
	if exceedsLimit(requested.MaxCPU, baseline.MaxCPU) {
		return false
	}
	if exceedsLimit(requested.MaxMemoryMB, baseline.MaxMemoryMB) {
		return false
	}
	if exceedsLimit(requested.MaxRuntimeSeconds, baseline.MaxRuntimeSeconds) {
		return false
	}
	return true
}

func subsetFold(values, allowed []string) bool {
	if len(values) == 0 {
		return true
	}
	if len(allowed) == 0 {
		return false
	}
	for _, value := range values {
		if !containsFoldString(allowed, value) {
			return false
		}
	}
	return true
}

func exceedsLimit(requested, baseline int) bool {
	if requested <= 0 {
		return false
	}
	if baseline <= 0 {
		return true
	}
	return requested > baseline
}

func intersectFold(left, right []string) []string {
	if len(left) == 0 {
		out := make([]string, 0, len(right))
		for _, value := range right {
			if strings.TrimSpace(value) != "" {
				out = append(out, value)
			}
		}
		return out
	}
	out := make([]string, 0, len(left))
	for _, value := range left {
		if strings.TrimSpace(value) == "" {
			continue
		}
		for _, candidate := range right {
			if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(candidate)) {
				out = append(out, value)
				break
			}
		}
	}
	return out
}
