package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/userconfig/config/secretscan"
)

var fallbackForbiddenSecretFieldNames = secretscan.ForbiddenSecretFieldNames

func readConfigFile(workspaceRoot, path string) ([]byte, error) {
	if ReadConfigFile != nil {
		return ReadConfigFile(workspaceRoot, path)
	}
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	path = strings.TrimSpace(path)
	if workspaceRoot == "" {
		return nil, fmt.Errorf("workspace root required")
	}
	if path == "" {
		return nil, fmt.Errorf("config path required")
	}

	absWorkspace, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	rel, err := filepath.Rel(absWorkspace, absPath)
	if err != nil {
		return nil, fmt.Errorf("check config path %q against workspace %q: %w", absPath, absWorkspace, err)
	}
	if rel == "." {
		return nil, fmt.Errorf("config path %q must reference a file", absPath)
	}
	if rel == secretscan.RuntimeStateDirName || strings.HasPrefix(rel, secretscan.RuntimeStateDirName+string(filepath.Separator)) {
		return nil, fmt.Errorf("config path %q is inside runtime state dir %q", absPath, secretscan.RuntimeStateDirName)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("config path %q is outside workspace root %q", absPath, absWorkspace)
	}

	return os.ReadFile(filepath.Clean(absPath))
}

func rejectForbiddenSecretFields(path string, data []byte) error {
	if RejectForbiddenSecretFields != nil {
		return RejectForbiddenSecretFields(path, data)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	var errs []error
	collectForbiddenSecretFields(&doc, path, nil, &errs)
	return joinErrors(errs)
}

func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	out := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			out = append(out, err)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return errors.Join(out...)
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
				*errs = append(*errs, fmt.Errorf("file=%s field=%s", path, strings.Join(nextPath, ".")))
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
	_, ok := fallbackForbiddenSecretFieldNames[normalized]
	return ok
}

func normalizeSecretFieldName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, " ", "")
	return name
}
