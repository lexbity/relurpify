package cfgload

import (
	"bytes"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// DecodeWithSchema validates the schema declaration, enforces registry support,
// rejects anchors and aliases, and decodes the body into out.
func DecodeWithSchema(path string, data []byte, registry *SchemaRegistry, out any) (SchemaDeclaration, error) {
	decl, body, err := SplitSchemaDocument(path, data)
	if err != nil {
		return SchemaDeclaration{}, err
	}
	if err := RejectForbiddenSecretFields(path, body); err != nil {
		return SchemaDeclaration{}, err
	}
	if registry == nil {
		registry = NewSchemaRegistry()
	}
	if err := registry.Require(decl); err != nil {
		return SchemaDeclaration{}, err
	}
	if err := decodeStrictYAMLBody(path, decl.Line, body, out); err != nil {
		return SchemaDeclaration{}, err
	}
	return decl, nil
}

func decodeStrictYAMLBody(path string, schemaLine int, body []byte, out any) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return &SchemaError{
			Path: path,
			Line: schemaLine + 1,
			Err:  fmt.Errorf("document body required"),
		}
	}

	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&doc); err != nil {
		if err == io.EOF {
			return &SchemaError{
				Path: path,
				Line: schemaLine + 1,
				Err:  fmt.Errorf("document body required"),
			}
		}
		return &SchemaError{
			Path: path,
			Line: schemaLine + 1,
			Err:  err,
		}
	}

	if err := rejectAnchors(&doc, path, schemaLine); err != nil {
		return err
	}

	if out == nil {
		return nil
	}

	dec = yaml.NewDecoder(bytes.NewReader(body))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		if err == io.EOF {
			return &SchemaError{
				Path: path,
				Line: schemaLine + 1,
				Err:  fmt.Errorf("document body required"),
			}
		}
		return &SchemaError{
			Path: path,
			Line: schemaLine + 1,
			Err:  err,
		}
	}
	return nil
}

func rejectAnchors(node *yaml.Node, path string, schemaLine int) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode {
		return &SchemaError{
			Path: path,
			Line: schemaLine + node.Line,
			Err:  ErrYAMLAnchorAlias,
		}
	}
	if node.Anchor != "" {
		return &SchemaError{
			Path: path,
			Line: schemaLine + node.Line,
			Err:  ErrYAMLAnchorAlias,
		}
	}
	for _, child := range node.Content {
		if err := rejectAnchors(child, path, schemaLine); err != nil {
			return err
		}
	}
	return nil
}
