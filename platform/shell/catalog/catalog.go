package catalog

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// ToolCatalogEntry is the canonical registry record for a shell tool.
type ToolCatalogEntry struct {
	Name            string
	Aliases         []string
	Family          string
	Intent          []string
	Description     string
	LongDescription string
	ParameterSchema ToolSchema
	OutputSchema    ToolSchema
	Preset          ToolPreset
	Tags            []string
	Deprecated      bool
	Replacement     string
	Examples        []ToolExample
}

// ToolPreset captures the executable form of a catalog entry.
type ToolPreset struct {
	CommandTemplate []string
	DefaultArgs     []string
	AllowStdin      bool
	SupportsWorkdir bool
	ResultStyle     string
}

// ToolSchema describes a structured input or output payload.
type ToolSchema struct {
	Type       string
	Properties map[string]ToolSchemaField
	Required   []string
	Items      *ToolSchemaField
}

// ToolSchemaField describes a field in a structured schema.
type ToolSchemaField struct {
	Type        string
	Description string
	Default     any
	Enum        []string
	Items       *ToolSchemaField
	Properties  map[string]ToolSchemaField
	Required    []string
}

// ToolExample shows a representative query/input/output shape.
type ToolExample struct {
	Query  string
	Input  map[string]any
	Output string
}

// CommandToolSpec describes a shell command wrapper in declarative form.
type CommandToolSpec struct {
	Name            string
	Aliases         []string
	Family          string
	Intent          []string
	Description     string
	LongDescription string
	CommandTemplate []string
	Command         string
	DefaultArgs     []string
	Tags            []string
	Examples        []ToolExample
	Deprecated      bool
	Replacement     string
	AllowStdin      bool
	SupportsWorkdir bool
	ResultStyle     string
	ParameterSchema  ToolSchema
	OutputSchema     ToolSchema
}

// SchemaIssue describes a single validation failure.
type SchemaIssue struct {
	Path    string
	Message string
}

// SchemaValidationError aggregates schema validation issues.
type SchemaValidationError struct {
	Issues []SchemaIssue
}

func (e *SchemaValidationError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Issues) == 0 {
		return "schema validation failed"
	}
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		if issue.Path == "" {
			parts = append(parts, issue.Message)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", issue.Path, issue.Message))
	}
	return strings.Join(parts, "; ")
}

// ToolCatalog stores canonical entries and alias mappings.
type ToolCatalog struct {
	entries map[string]ToolCatalogEntry
	aliases map[string]string
}

// NewToolCatalog builds an empty catalog.
func NewToolCatalog() *ToolCatalog {
	return &ToolCatalog{
		entries: make(map[string]ToolCatalogEntry),
		aliases: make(map[string]string),
	}
}

// NormalizeName canonicalizes a tool or alias name for registry lookup.
func NormalizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastUnderscore = false
		case r == '_' || r == '-' || r == '.' || r == '/' || unicode.IsSpace(r):
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	return strings.ReplaceAll(out, "__", "_")
}

// Register inserts an entry and its aliases into the catalog.
func (c *ToolCatalog) Register(entry ToolCatalogEntry) error {
	if c == nil {
		return fmt.Errorf("catalog missing")
	}
	canonical := NormalizeName(entry.Name)
	if canonical == "" {
		return fmt.Errorf("tool name missing")
	}
	entry.Name = canonical
	if err := entry.Validate(); err != nil {
		return err
	}
	if _, exists := c.entries[canonical]; exists {
		return fmt.Errorf("tool %q already registered", canonical)
	}
	c.entries[canonical] = entry
	c.aliases[canonical] = canonical

	for _, alias := range entry.Aliases {
		normalized := NormalizeName(alias)
		if normalized == "" || normalized == canonical {
			continue
		}
		if existing, ok := c.aliases[normalized]; ok && existing != canonical {
			return fmt.Errorf("alias %q already maps to %q", alias, existing)
		}
		c.aliases[normalized] = canonical
	}
	return nil
}

// EntryFromCommandSpec converts a declarative command spec into a catalog entry.
func EntryFromCommandSpec(spec CommandToolSpec) ToolCatalogEntry {
	return ToolCatalogEntry{
		Name:            NormalizeName(spec.Name),
		Aliases:         normalizeNames(spec.Aliases),
		Family:          NormalizeName(spec.Family),
		Intent:          normalizeNames(spec.Intent),
		Description:     spec.Description,
		LongDescription: spec.LongDescription,
		ParameterSchema: schemaOrDefault(spec.ParameterSchema, ToolSchema{Type: "object"}),
		OutputSchema:    schemaOrDefault(spec.OutputSchema, ToolSchema{Type: "object"}),
		Preset: ToolPreset{
			CommandTemplate: normalizeCommandTemplate(spec.CommandTemplate, spec.Command),
			DefaultArgs:     append([]string(nil), spec.DefaultArgs...),
			AllowStdin:      true,
			SupportsWorkdir: true,
			ResultStyle:     spec.ResultStyle,
		},
		Tags:        normalizeNames(spec.Tags),
		Deprecated:   spec.Deprecated,
		Replacement:  NormalizeName(spec.Replacement),
		Examples:     append([]ToolExample(nil), spec.Examples...),
	}
}

// Lookup resolves a canonical name or alias.
func (c *ToolCatalog) Lookup(name string) (ToolCatalogEntry, bool) {
	if c == nil {
		return ToolCatalogEntry{}, false
	}
	normalized := NormalizeName(name)
	if normalized == "" {
		return ToolCatalogEntry{}, false
	}
	if canonical, ok := c.aliases[normalized]; ok {
		entry, exists := c.entries[canonical]
		return entry, exists
	}
	entry, ok := c.entries[normalized]
	return entry, ok
}

// List returns entries in deterministic canonical order.
func (c *ToolCatalog) List() []ToolCatalogEntry {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.entries))
	for name := range c.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]ToolCatalogEntry, 0, len(names))
	for _, name := range names {
		out = append(out, c.entries[name])
	}
	return out
}

// Validate checks the entry for internal consistency.
func (e ToolCatalogEntry) Validate() error {
	var issues []SchemaIssue
	if e.Name == "" {
		issues = append(issues, SchemaIssue{Path: "name", Message: "tool name missing"})
	}
	if err := e.ParameterSchema.ValidatePath("parameter_schema"); err != nil {
		issues = append(issues, err.Issues...)
	}
	if err := e.OutputSchema.ValidatePath("output_schema"); err != nil {
		issues = append(issues, err.Issues...)
	}
	if e.Deprecated && e.Replacement != "" && NormalizeName(e.Replacement) == "" {
		issues = append(issues, SchemaIssue{Path: "replacement", Message: "replacement name is invalid"})
	}
	if len(issues) > 0 {
		return &SchemaValidationError{Issues: issues}
	}
	return nil
}

// Validate checks a schema and returns path-aware validation errors.
func (s ToolSchema) Validate() error {
	return s.ValidatePath("schema")
}

// ValidatePath validates the schema using a caller-provided path prefix.
func (s ToolSchema) ValidatePath(prefix string) *SchemaValidationError {
	var issues []SchemaIssue
	validateSchema(&issues, prefix, s)
	if len(issues) == 0 {
		return nil
	}
	return &SchemaValidationError{Issues: issues}
}

func validateSchema(issues *[]SchemaIssue, path string, schema ToolSchema) {
	if schema.Type == "" {
		*issues = append(*issues, SchemaIssue{Path: path + ".type", Message: "schema type missing"})
		return
	}
	if schema.Type == "array" && schema.Items == nil {
		*issues = append(*issues, SchemaIssue{Path: path + ".items", Message: "array schema missing items"})
	}
	if schema.Type == "object" {
		validateRequiredFields(issues, path, schema.Required, schema.Properties)
	}
	for name, field := range schema.Properties {
		validateField(issues, path+".properties."+name, field)
	}
	if schema.Items != nil {
		validateField(issues, path+".items", *schema.Items)
	}
}

func validateRequiredFields(issues *[]SchemaIssue, path string, required []string, props map[string]ToolSchemaField) {
	if len(required) == 0 {
		return
	}
	for idx, name := range required {
		if name == "" {
			*issues = append(*issues, SchemaIssue{
				Path:    fmt.Sprintf("%s.required[%d]", path, idx),
				Message: "required field name missing",
			})
			continue
		}
		if _, ok := props[name]; !ok {
			*issues = append(*issues, SchemaIssue{
				Path:    fmt.Sprintf("%s.required[%d]", path, idx),
				Message: fmt.Sprintf("required field %q missing from properties", name),
			})
		}
	}
}

func validateField(issues *[]SchemaIssue, path string, field ToolSchemaField) {
	if field.Type == "" {
		*issues = append(*issues, SchemaIssue{Path: path + ".type", Message: "field type missing"})
		return
	}
	if len(field.Enum) > 0 && field.Default != nil {
		if !containsString(field.Enum, fmt.Sprint(field.Default)) {
			*issues = append(*issues, SchemaIssue{Path: path + ".default", Message: "default not present in enum"})
		}
	}
	if field.Type == "array" {
		if field.Items == nil {
			*issues = append(*issues, SchemaIssue{Path: path + ".items", Message: "array field missing items"})
		} else {
			validateField(issues, path+".items", *field.Items)
		}
	}
	if field.Type == "object" {
		validateRequiredFields(issues, path, field.Required, field.Properties)
		for name, nested := range field.Properties {
			validateField(issues, path+".properties."+name, nested)
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func normalizeNames(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := NormalizeName(value); normalized != "" {
			out = append(out, normalized)
		}
	}
	return out
}

func normalizeCommandTemplate(template []string, command string) []string {
	if len(template) > 0 {
		out := make([]string, 0, len(template))
		for _, part := range template {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			out = append(out, part)
		}
		return out
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	return []string{command}
}

func schemaOrDefault(schema ToolSchema, fallback ToolSchema) ToolSchema {
	if schema.Type == "" {
		return fallback
	}
	return schema
}
