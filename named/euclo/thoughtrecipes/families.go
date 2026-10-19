package thoughtrecipe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// FamiliesFileName is the workspace-data file mapping classified intent
// families to the thoughtrecipe IDs their clarification handoffs target. It
// lives in relurpify_cfg/euclo next to the recipes and is user-editable
// workspace data: a materialization adds it once and never overwrites an
// edit, and it — not a compiled-in table — decides which recipe a family's
// handoff names.
const FamiliesFileName = "families.yaml"

// familiesFile is the on-disk schema of FamiliesFileName.
type familiesFile struct {
	Schema   string            `yaml:"schema"`
	Handoffs map[string]string `yaml:"handoffs"`
}

// familiesFileSchema is the only accepted schema statement.
const familiesFileSchema = "relurpify/euclo/families/v1"

// parseFamiliesFile decodes and validates a families.yaml body: keys are
// lowercased families, values are non-empty recipe IDs.
func parseFamiliesFile(body []byte) (map[string]string, error) {
	var parsed familiesFile
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", FamiliesFileName, err)
	}
	if strings.TrimSpace(parsed.Schema) != familiesFileSchema {
		return nil, fmt.Errorf("%s: unsupported schema %q (want %q)", FamiliesFileName, parsed.Schema, familiesFileSchema)
	}
	handoffs := make(map[string]string, len(parsed.Handoffs))
	for family, target := range parsed.Handoffs {
		family = strings.ToLower(strings.TrimSpace(family))
		target = strings.TrimSpace(target)
		if family == "" || target == "" {
			return nil, fmt.Errorf("%s: handoff entries need a family and a recipe ID", FamiliesFileName)
		}
		handoffs[family] = target
	}
	return handoffs, nil
}

// LoadHandoffsFile reads the workspace handoff table. A missing file is an
// empty table, not an error: handoff resolution degrades to its named
// runtime failure ("no thoughtrecipe mapped for family") instead of coupling
// every workspace to a shipped file it may have deliberately removed.
func LoadHandoffsFile(root string) (map[string]string, error) {
	// #nosec G304 -- root is a workspace path supplied by the operator; FamiliesFileName is a compile-time constant, so the path is not attacker-controlled.
	body, err := os.ReadFile(filepath.Join(root, FamiliesFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", FamiliesFileName, err)
	}
	return parseFamiliesFile(body)
}
