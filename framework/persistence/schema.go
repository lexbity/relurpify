package persistence

import "time"

// SchemaMetadata is attached to every persisted record that needs migration or replay support.
type SchemaMetadata struct {
	SchemaName    string    `json:"schema_name"`
	SchemaVersion int       `json:"schema_version"`
	EntityKind    string    `json:"entity_kind"`
	EntityID      string    `json:"entity_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	SourcePackage string    `json:"source_package"`
}

// SchemaVersion represents a versioned schema.
type SchemaVersion struct {
	Name    string
	Version int
}

// CurrentSchemaVersions returns the current schema versions for all entity kinds.
func CurrentSchemaVersions() map[string]SchemaVersion {
	return map[string]SchemaVersion{
		"workflow":                  {Name: "workflow", Version: 1},
		"workflow_run":              {Name: "workflow_run", Version: 1},
		"delegation":               {Name: "delegation", Version: 1},
		"delegation_transition":    {Name: "delegation_transition", Version: 1},
		"workflow_event":            {Name: "workflow_event", Version: 1},
		"workflow_artifact":        {Name: "workflow_artifact", Version: 1},
		"lineage_binding":           {Name: "lineage_binding", Version: 1},
		"compilation_record":       {Name: "compilation_record", Version: 1},
		"compilation_artifact":     {Name: "compilation_artifact", Version: 1},
		"compilation_cache_entry":  {Name: "compilation_cache_entry", Version: 1},
	}
}

// NewSchemaMetadata creates schema metadata for a given entity.
func NewSchemaMetadata(entityKind, entityID, sourcePackage string) SchemaMetadata {
	versions := CurrentSchemaVersions()
	schema, ok := versions[entityKind]
	if !ok {
		schema = SchemaVersion{Name: entityKind, Version: 1}
	}

	now := time.Now().UTC()
	return SchemaMetadata{
		SchemaName:    schema.Name,
		SchemaVersion: schema.Version,
		EntityKind:    entityKind,
		EntityID:      entityID,
		CreatedAt:     now,
		UpdatedAt:     now,
		SourcePackage: sourcePackage,
	}
}

// Validate checks if the schema metadata is valid.
func (m SchemaMetadata) Validate() error {
	if m.SchemaName == "" {
		return &ValidationError{Field: "schema_name", Message: "schema_name is required"}
	}
	if m.SchemaVersion <= 0 {
		return &ValidationError{Field: "schema_version", Message: "schema_version must be positive"}
	}
	if m.EntityKind == "" {
		return &ValidationError{Field: "entity_kind", Message: "entity_kind is required"}
	}
	if m.EntityID == "" {
		return &ValidationError{Field: "entity_id", Message: "entity_id is required"}
	}
	if m.SourcePackage == "" {
		return &ValidationError{Field: "source_package", Message: "source_package is required"}
	}
	return nil
}

// ValidationError represents a schema validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
