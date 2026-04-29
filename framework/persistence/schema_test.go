package persistence

import (
	"testing"
	"time"
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
			entityKind:    "workflow",
			entityID:      "wf-123",
			sourcePackage: "framework/agentlifecycle",
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
		name    string
		metadata SchemaMetadata
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid metadata",
			metadata: SchemaMetadata{
				SchemaName:    "workflow",
				SchemaVersion: 1,
				EntityKind:    "workflow",
				EntityID:      "wf-123",
				CreatedAt:     now,
				UpdatedAt:     now,
				SourcePackage: "framework/agentlifecycle",
			},
			wantErr: false,
		},
		{
			name: "missing schema name",
			metadata: SchemaMetadata{
				SchemaVersion: 1,
				EntityKind:    "workflow",
				EntityID:      "wf-123",
				CreatedAt:     now,
				UpdatedAt:     now,
				SourcePackage: "framework/agentlifecycle",
			},
			wantErr: true,
			errMsg:  "schema_name",
		},
		{
			name: "invalid schema version",
			metadata: SchemaMetadata{
				SchemaName:    "workflow",
				SchemaVersion: 0,
				EntityKind:    "workflow",
				EntityID:      "wf-123",
				CreatedAt:     now,
				UpdatedAt:     now,
				SourcePackage: "framework/agentlifecycle",
			},
			wantErr: true,
			errMsg:  "schema_version",
		},
		{
			name: "missing entity kind",
			metadata: SchemaMetadata{
				SchemaName:    "workflow",
				SchemaVersion: 1,
				EntityID:      "wf-123",
				CreatedAt:     now,
				UpdatedAt:     now,
				SourcePackage: "framework/agentlifecycle",
			},
			wantErr: true,
			errMsg:  "entity_kind",
		},
		{
			name: "missing entity id",
			metadata: SchemaMetadata{
				SchemaName:    "workflow",
				SchemaVersion: 1,
				EntityKind:    "workflow",
				CreatedAt:     now,
				UpdatedAt:     now,
				SourcePackage: "framework/agentlifecycle",
			},
			wantErr: true,
			errMsg:  "entity_id",
		},
		{
			name: "missing source package",
			metadata: SchemaMetadata{
				SchemaName:    "workflow",
				SchemaVersion: 1,
				EntityKind:    "workflow",
				EntityID:      "wf-123",
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
				if _, ok := err.(*ValidationError); !ok {
					t.Errorf("Error should be ValidationError, got %T", err)
				}
				if !contains(err.Error(), tt.errMsg) {
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
		"workflow",
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
