package persistence

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	Frameworkagentlifecycle_schema_test = "framework/agentlifecycle"
	Wf123_schema_test                   = "wf-123"
	Workflow_schema_test                = "workflow"
)

func TestNewSchemaMetadata(t *testing.T) {
	tests := []struct {
		name          string
		entityKind    string
		entityID      string
		sourcePackage string
		wantErr       bool
	}{
		{
			name:          "valid metadata",
			entityKind:    Workflow_schema_test,
			entityID:      Wf123_schema_test,
			sourcePackage: Frameworkagentlifecycle_schema_test,
			wantErr:       false,
		},
		{
			name:          "unknown entity kind uses default version",
			entityKind:    "unknown_kind",
			entityID:      "ent-456",
			sourcePackage: "framework/compiler",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := NewSchemaMetadata(tt.entityKind, tt.entityID, tt.sourcePackage)

			if metadata.SchemaName == "" {
				t.Error("SchemaName should not be empty")
			}
			if metadata.SchemaVersion <= 0 {
				t.Error("SchemaVersion should be positive")
			}
			if metadata.EntityKind != tt.entityKind {
				t.Errorf("EntityKind = %v, want %v", metadata.EntityKind, tt.entityKind)
			}
			if metadata.EntityID != tt.entityID {
				t.Errorf("EntityID = %v, want %v", metadata.EntityID, tt.entityID)
			}
			if metadata.SourcePackage != tt.sourcePackage {
				t.Errorf("SourcePackage = %v, want %v", metadata.SourcePackage, tt.sourcePackage)
			}
			if metadata.CreatedAt.IsZero() {
				t.Error("CreatedAt should not be zero")
			}
			if metadata.UpdatedAt.IsZero() {
				t.Error("UpdatedAt should not be zero")
			}
		})
	}
}

func TestSchemaMetadataValidate(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name     string
		metadata SchemaMetadata
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid metadata",
			metadata: SchemaMetadata{
				SchemaName:    Workflow_schema_test,
				SchemaVersion: 1,
				EntityKind:    Workflow_schema_test,
				EntityID:      Wf123_schema_test,
				CreatedAt:     now,
				UpdatedAt:     now,
				SourcePackage: Frameworkagentlifecycle_schema_test,
			},
			wantErr: false,
		},
		{
			name: "missing schema name",
			metadata: SchemaMetadata{
				SchemaVersion: 1,
				EntityKind:    Workflow_schema_test,
				EntityID:      Wf123_schema_test,
				CreatedAt:     now,
				UpdatedAt:     now,
				SourcePackage: Frameworkagentlifecycle_schema_test,
			},
			wantErr: true,
			errMsg:  "schema_name",
		},
		{
			name: "invalid schema version",
			metadata: SchemaMetadata{
				SchemaName:    Workflow_schema_test,
				SchemaVersion: 0,
				EntityKind:    Workflow_schema_test,
				EntityID:      Wf123_schema_test,
				CreatedAt:     now,
				UpdatedAt:     now,
				SourcePackage: Frameworkagentlifecycle_schema_test,
			},
			wantErr: true,
			errMsg:  "schema_version",
		},
		{
			name: "missing entity kind",
			metadata: SchemaMetadata{
				SchemaName:    Workflow_schema_test,
				SchemaVersion: 1,
				EntityID:      Wf123_schema_test,
				CreatedAt:     now,
				UpdatedAt:     now,
				SourcePackage: Frameworkagentlifecycle_schema_test,
			},
			wantErr: true,
			errMsg:  "entity_kind",
		},
		{
			name: "missing entity id",
			metadata: SchemaMetadata{
				SchemaName:    Workflow_schema_test,
				SchemaVersion: 1,
				EntityKind:    Workflow_schema_test,
				CreatedAt:     now,
				UpdatedAt:     now,
				SourcePackage: Frameworkagentlifecycle_schema_test,
			},
			wantErr: true,
			errMsg:  "entity_id",
		},
		{
			name: "missing source package",
			metadata: SchemaMetadata{
				SchemaName:    Workflow_schema_test,
				SchemaVersion: 1,
				EntityKind:    Workflow_schema_test,
				EntityID:      Wf123_schema_test,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			wantErr: true,
			errMsg:  "source_package",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.metadata.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" {
				var validationError *ValidationError
				if !errors.As(err, &validationError) {
					t.Errorf("Error should be ValidationError, got %T", err)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Error message should contain %q, got %q", tt.errMsg, err.Error())
				}
			}
		})
	}
}

func TestCurrentSchemaVersions(t *testing.T) {
	versions := CurrentSchemaVersions()

	if len(versions) == 0 {
		t.Error("CurrentSchemaVersions should return non-empty map")
	}

	expectedKinds := []string{
		Workflow_schema_test,
		"workflow_run",
		"delegation",
		"delegation_transition",
		"workflow_event",
		"workflow_artifact",
		"lineage_binding",
		"compilation_record",
		"compilation_artifact",
		"compilation_cache_entry",
	}

	for _, kind := range expectedKinds {
		if _, ok := versions[kind]; !ok {
			t.Errorf("CurrentSchemaVersions should contain kind %q", kind)
		}
	}
}
