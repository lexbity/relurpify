package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const validManifestYAML = `apiVersion: relurpify.io/v1
kind: AgentManifest
metadata:
  name: test-agent
  version: "1.2.3"
spec:
  agent:
    model:
      name: test-model
    prompt: "You are a test agent."
  permissions:
    file:
      read:
        - relurpify_cfg
  resources:
    max_tokens: 1024
  security:
    read_only_root: false
  context:
    max_tokens: 4096
`

// parseTestDocument builds a config.Document the way the loader does: the
// top-level mapping drives the envelope, and every key under `spec` becomes a
// section node. Sections are never decoded here — that is AssembleContract's
// job.
func parseTestDocument(t *testing.T, raw string) *config.Document {
	t.Helper()
	doc, err := parseDocumentBytes(raw)
	require.NoError(t, err)
	return doc
}

func parseDocumentBytes(raw string) (*config.Document, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		return nil, err
	}
	doc := &config.Document{
		APIVersion: "relurpify.io/v1",
		Kind:       "AgentManifest",
		Spec:       map[string]yaml.Node{},
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return doc, nil
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return doc, nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := mapping.Content[i].Value
		value := mapping.Content[i+1]
		switch key {
		case "apiVersion":
			doc.APIVersion = value.Value
		case "kind":
			doc.Kind = value.Value
		case "metadata":
			var meta config.DocumentMetadata
			if err := value.Decode(&meta); err == nil {
				doc.Metadata = meta
			}
		case "spec":
			if value.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(value.Content); j += 2 {
					doc.Spec[value.Content[j].Value] = *value.Content[j+1]
				}
			}
		}
	}
	return doc, nil
}

func TestAssembleContractBuildsEffectiveContractFromManifest(t *testing.T) {
	doc := parseTestDocument(t, validManifestYAML)

	contract, err := AssembleContract(doc)
	require.NoError(t, err)

	assert.Equal(t, "test-agent", contract.AgentID)
	require.NotNil(t, contract.AgentSpec)
	assert.Equal(t, "test-model", contract.AgentSpec.Model.Name)
	assert.Equal(t, "test-agent", contract.Sources.ManifestName)
	assert.Equal(t, "1.2.3", contract.Sources.ManifestVersion)
}

func TestAssembleContractRejectsNilDocument(t *testing.T) {
	_, err := AssembleContract(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "document required")
}

func TestAssembleContractRejectsMissingAgentSection(t *testing.T) {
	doc := parseTestDocument(t, `apiVersion: relurpify.io/v1
kind: AgentManifest
metadata:
  name: test-agent
`)

	_, err := AssembleContract(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent section not found")
}

func TestAssembleContractRejectsMalformedAgentSection(t *testing.T) {
	doc := parseTestDocument(t, `metadata:
  name: test-agent
spec:
  agent: 42
`)

	_, err := AssembleContract(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode agent")
}

func TestAssembleContractRejectsMalformedPermissionsSection(t *testing.T) {
	doc := parseTestDocument(t, `metadata:
  name: test-agent
spec:
  agent:
    model:
      name: m
  permissions: 42
`)

	_, err := AssembleContract(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode permissions")
}

func TestAssembleContractRejectsMalformedResourcesSection(t *testing.T) {
	doc := parseTestDocument(t, `metadata:
  name: test-agent
spec:
  agent:
    model:
      name: m
  resources: 42
`)

	_, err := AssembleContract(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode resources")
}

func TestAssembleContractRejectsMalformedSecuritySection(t *testing.T) {
	doc := parseTestDocument(t, `metadata:
  name: test-agent
spec:
  agent:
    model:
      name: m
  security: 42
`)

	_, err := AssembleContract(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode security")
}

func TestAssembleContractToleratesMalformedContextSection(t *testing.T) {
	// The context policy section is best-effort: a malformed section is
	// ignored, never fatal.
	doc := parseTestDocument(t, `metadata:
  name: test-agent
spec:
  agent:
    model:
      name: m
  context: 42
`)

	contract, err := AssembleContract(doc)
	require.NoError(t, err)
	assert.Equal(t, "m", contract.AgentSpec.Model.Name)
}

func TestDecodeYAMLNodeDecodesTypedValues(t *testing.T) {
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(`name: probe
count: 3
`), &node))

	payload, err := DecodeYAMLNode[struct {
		Name  string `yaml:"name"`
		Count int    `yaml:"count"`
	}](node)
	require.NoError(t, err)
	assert.Equal(t, "probe", payload.Name)
	assert.Equal(t, 3, payload.Count)
}

func TestDecodeYAMLNodeRejectsZeroNode(t *testing.T) {
	_, err := DecodeYAMLNode[string](yaml.Node{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "yaml node is zero-valued")
}

func TestDecodeYAMLNodeRejectsTypeMismatch(t *testing.T) {
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(`count: not-a-number`), &node))

	_, err := DecodeYAMLNode[struct {
		Count int `yaml:"count"`
	}](node)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode yaml node")
}

// FuzzAssembleContract drives the manifest assembly over arbitrary untrusted
// YAML documents. Assembly must never panic, and any returned contract must
// uphold its invariants: a non-nil spec and a non-empty agent ID.
func FuzzAssembleContract(f *testing.F) {
	f.Add(validManifestYAML)
	f.Add("spec:\n  agent: {}\n")
	f.Add("spec:\n  agent: 42\n")
	f.Add("spec:\n  agent:\n    model:\n      name: [1, 2, 3]\n")
	f.Add("metadata:\n  name: x\nspec: not-a-mapping\n")
	f.Add("")
	f.Add(":::\n")
	f.Add("spec:\n  agent: !<tag:broken> {a: b}\n")
	f.Add(strings.Repeat("spec:\n  agent:\n    model:\n      name: m\n", 32))

	f.Fuzz(func(t *testing.T, raw string) {
		doc, err := parseDocumentBytes(raw)
		if err != nil {
			return // unparseable YAML: the loader rejects before assembly
		}
		contract, err := AssembleContract(doc)
		if err != nil {
			return
		}
		if contract.AgentSpec == nil {
			t.Errorf("AssembleContract returned nil AgentSpec without error for input: %q", raw)
		}
		if contract.AgentID == "" {
			t.Errorf("AssembleContract returned empty AgentID without error for input: %q", raw)
		}
	})
}
