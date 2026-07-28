package config

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/userconfig/config/secretscan"
)

// RejectForbiddenSecretFields scans a YAML document for secret-bearing field names.
func RejectForbiddenSecretFields(path string, data []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	var errs []error
	collectForbiddenSecretFields(&doc, path, nil, &errs)
	return errors.Join(errs...)
}

func collectForbiddenSecretFields(node *yaml.Node, path string, fieldPath []string, errs *[]error) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			collectForbiddenSecretFields(child, path, fieldPath, errs)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			key := strings.TrimSpace(keyNode.Value)
			nextPath := append(append([]string(nil), fieldPath...), key)
			if isForbiddenSecretFieldName(key) {
				*errs = append(*errs, &SecretFieldError{
					Path:  path,
					Field: strings.Join(nextPath, "."),
					Hint:  secretFieldHint(key),
				})
			}
			collectForbiddenSecretFields(valNode, path, nextPath, errs)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			nextPath := append(append([]string(nil), fieldPath...), fmt.Sprintf("[%d]", i))
			collectForbiddenSecretFields(child, path, nextPath, errs)
		}
	}
}

func isForbiddenSecretFieldName(name string) bool {
	normalized := normalizeSecretFieldName(name)
	_, ok := secretscan.ForbiddenSecretFieldNames[normalized]
	return ok
}

func normalizeSecretFieldName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, " ", "")
	return name
}

func secretFieldHint(name string) string {
	switch normalizeSecretFieldName(name) {
	case "apikey", "apisecret", "privatekey":
		return "Use environment variable RELURPIFY_LLM_API_KEY instead."
	default:
		return "Use an environment variable instead."
	}
}
