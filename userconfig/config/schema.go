package config

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const schemaPrefix = "relurpify"

var schemaLinePattern = regexp.MustCompile(`^` + schemaPrefix + `/((?:[a-z0-9][a-z0-9_-]*)(?:/(?:[a-z0-9][a-z0-9_-]*))*)/v([1-9][0-9]*)$`)

// SchemaDeclaration records the first-line schema contract for a config file.
type SchemaDeclaration struct {
	Raw     string
	Kind    string
	Version int
	Line    int
}

func (d SchemaDeclaration) String() string {
	if d.Raw != "" {
		return d.Raw
	}
	if d.Kind == "" || d.Version == 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/v%d", schemaPrefix, d.Kind, d.Version)
}

// SplitSchemaDocument extracts the schema declaration from the first non-comment
// line and returns the remaining document body.
func SplitSchemaDocument(path string, data []byte) (SchemaDeclaration, []byte, error) {
	lines := bytes.Split(data, []byte("\n"))
	for idx, raw := range lines {
		line := strings.TrimSpace(strings.TrimSuffix(string(raw), "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		decl, err := parseSchemaLine(path, idx+1, line)
		if err != nil {
			return SchemaDeclaration{}, nil, err
		}
		body := bytes.Join(lines[idx+1:], []byte("\n"))
		return decl, body, nil
	}
	return SchemaDeclaration{}, nil, &SchemaError{
		Path: path,
		Err:  ErrMissingSchemaDeclaration,
	}
}

func parseSchemaLine(path string, lineNumber int, line string) (SchemaDeclaration, error) {
	var raw struct {
		Schema string `yaml:"schema"`
	}
	if err := yaml.Unmarshal([]byte(line), &raw); err != nil {
		return SchemaDeclaration{}, &SchemaError{
			Path: path,
			Line: lineNumber,
			Err:  ErrInvalidSchemaDeclaration,
		}
	}
	value := strings.TrimSpace(raw.Schema)
	if value == "" {
		return SchemaDeclaration{}, &SchemaError{
			Path: path,
			Line: lineNumber,
			Err:  ErrMissingSchemaDeclaration,
		}
	}
	match := schemaLinePattern.FindStringSubmatch(value)
	if match == nil {
		return SchemaDeclaration{}, &SchemaError{
			Path:   path,
			Line:   lineNumber,
			Schema: value,
			Err:    ErrInvalidSchemaDeclaration,
		}
	}
	version, err := strconv.Atoi(match[2])
	if err != nil {
		return SchemaDeclaration{}, &SchemaError{
			Path:   path,
			Line:   lineNumber,
			Schema: value,
			Err:    ErrInvalidSchemaDeclaration,
		}
	}
	return SchemaDeclaration{
		Raw:     value,
		Kind:    match[1],
		Version: version,
		Line:    lineNumber,
	}, nil
}
