package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

// MarshalWithSchema prepends the canonical schema declaration to a YAML body.
func MarshalWithSchema(schema string, v any) ([]byte, error) {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return nil, fmt.Errorf("schema required")
	}
	body, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(schema)+len(body)+16)
	out = append(out, []byte("schema: ")...)
	out = append(out, schema...)
	out = append(out, '\n')
	if len(body) > 0 {
		out = append(out, body...)
		if body[len(body)-1] != '\n' {
			out = append(out, '\n')
		}
	}
	return out, nil
}

// WriteWithSchema writes a YAML document with a leading schema declaration.
func WriteWithSchema(path, schema string, v any) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path required")
	}
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := MarshalWithSchema(schema, v)
	if err != nil {
		return err
	}
	return fs.WriteFileSecure(path, data)
}
