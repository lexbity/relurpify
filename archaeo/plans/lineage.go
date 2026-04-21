package plans

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	"codeburg.org/lexbit/relurpify/archaeo/internal/storeutil"
)

type Lineage struct {
	WorkflowID       string
	Versions         []archaeodomain.VersionedLivingPlan
	ActiveVersion    *archaeodomain.VersionedLivingPlan
	DraftVersions    []archaeodomain.VersionedLivingPlan
	LatestDraft      *archaeodomain.VersionedLivingPlan
	RecomputePending bool
}

func (s Service) LoadLineage(ctx context.Context, workflowID string) (*Lineage, error) {
	versions, err := s.ListVersions(ctx, workflowID)
	if err != nil || strings.TrimSpace(workflowID) == "" {
		return nil, err
	}
	lineage := &Lineage{
		WorkflowID: strings.TrimSpace(workflowID),
		Versions:   append([]archaeodomain.VersionedLivingPlan(nil), versions...),
	}
	for i := range versions {
		record := versions[i]
		if record.Status == archaeodomain.LivingPlanVersionActive {
			copy := record
			lineage.ActiveVersion = &copy
		}
		if record.Status == archaeodomain.LivingPlanVersionDraft {
			lineage.DraftVersions = append(lineage.DraftVersions, record)
			copy := record
			lineage.LatestDraft = &copy
		}
		if record.RecomputeRequired {
			lineage.RecomputePending = true
		}
	}
	return lineage, nil
}

func (s Service) LoadActiveVersion(ctx context.Context, workflowID string) (*archaeodomain.VersionedLivingPlan, error) {
	lineage, err := s.LoadLineage(ctx, workflowID)
	if err != nil || lineage == nil {
		return nil, err
	}
	return lineage.ActiveVersion, nil
}

func (s Service) ListVersions(ctx context.Context, workflowID string) ([]archaeodomain.VersionedLivingPlan, error) {
	store := s.workflowStore()
	if store == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	artifacts, err := storeutil.ListWorkflowArtifactsByKind(ctx, store, workflowID, "", versionArtifactKind)
	if err != nil {
		return nil, err
	}
	out := make([]archaeodomain.VersionedLivingPlan, 0, len(artifacts))
	for _, artifact := range artifacts {
		var record archaeodomain.VersionedLivingPlan
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &record); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	sortVersions(out)
	return out, nil
}

func (s Service) LoadVersion(ctx context.Context, workflowID string, version int) (*archaeodomain.VersionedLivingPlan, error) {
	store := s.workflowStore()
	if store == nil || strings.TrimSpace(workflowID) == "" || version <= 0 {
		return nil, nil
	}
	if artifact, ok, err := storeutil.WorkflowArtifactByID(ctx, store, fmt.Sprintf("archaeo-plan-version:%s:%d", strings.TrimSpace(workflowID), version)); err != nil {
		return nil, err
	} else if ok && artifact != nil {
		var record archaeodomain.VersionedLivingPlan
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &record); err != nil {
			return nil, err
		}
		return &record, nil
	}
	versions, err := s.ListVersions(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	for i := range versions {
		if versions[i].Version == version {
			record := versions[i]
			return &record, nil
		}
	}
	return nil, nil
}

func sortVersions(values []archaeodomain.VersionedLivingPlan) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j].Version < values[i].Version {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
