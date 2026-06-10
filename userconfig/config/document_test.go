package config

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// sampleAgentYAML is a minimal agent config with a permissions section, used
// to verify Document loading, section access, and fingerprint stability.
const sampleAgentYAML = `apiVersion: relurpify.io/v1
kind: AgentManifest
metadata:
  name: test-agent
  version: "1.0.0"
  description: Test agent for document loading
spec:
  permissions:
    filesystem:
      - action: read
        path: /workspace/**
      - action: write
        path: /workspace/output/**
    executables:
      - binary: echo
        action: allow
  context:
    ingestion:
      max_tokens: 4096
      exclude_patterns:
        - "*.log"
`

func TestLoadDocument_basicEnvelope(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(sampleAgentYAML), 0644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}

	if snapshot.Document.APIVersion != "relurpify.io/v1" {
		t.Errorf("APIVersion = %q, want %q", snapshot.Document.APIVersion, "relurpify.io/v1")
	}
	if snapshot.Document.Kind != "AgentManifest" {
		t.Errorf("Kind = %q, want %q", snapshot.Document.Kind, "AgentManifest")
	}
	if snapshot.Document.Metadata.Name != "test-agent" {
		t.Errorf("Metadata.Name = %q, want %q", snapshot.Document.Metadata.Name, "test-agent")
	}
	if snapshot.SourcePath != path {
		t.Errorf("SourcePath = %q, want %q", snapshot.SourcePath, path)
	}
	if snapshot.Document == nil {
		t.Fatal("Document is nil")
	}
}

func TestLoadDocument_sections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(sampleAgentYAML), 0644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}

	// Verify permissions section exists
	permNode, ok := snapshot.Document.Section("permissions")
	if !ok {
		t.Fatal("expected 'permissions' section to exist")
	}
	var permSection map[string]any
	if err := permNode.Decode(&permSection); err != nil {
		t.Fatalf("decode permissions section: %v", err)
	}
	if _, ok := permSection["filesystem"]; !ok {
		t.Error("permissions section missing 'filesystem' key")
	}

	// Verify context section exists
	ctxNode, ok := snapshot.Document.Section("context")
	if !ok {
		t.Fatal("expected 'context' section to exist")
	}
	_ = ctxNode

	// Verify non-existent section returns false
	_, ok = snapshot.Document.Section("nonexistent")
	if ok {
		t.Error("expected 'nonexistent' section to not exist")
	}
}

func TestLoadDocument_fingerprintStability(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(sampleAgentYAML), 0644); err != nil {
		t.Fatal(err)
	}

	snapshot1, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument (1): %v", err)
	}
	snapshot2, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument (2): %v", err)
	}

	// Same bytes must produce identical fingerprints
	if snapshot1.Fingerprint != snapshot2.Fingerprint {
		t.Error("fingerprint changed between identical loads")
	}

	// Fingerprint must match sha256 of raw bytes
	expected := sha256.Sum256([]byte(sampleAgentYAML))
	if snapshot1.Fingerprint != expected {
		t.Errorf("fingerprint = %x, want %x", snapshot1.Fingerprint, expected)
	}
}

func TestLoadDocument_fingerprintChangesOnModification(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(sampleAgentYAML), 0644); err != nil {
		t.Fatal(err)
	}

	snapshot1, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument (1): %v", err)
	}

	// Modify the file and re-load
	modified := strings.ReplaceAll(sampleAgentYAML, "test-agent", "modified-agent")
	if err := os.WriteFile(path, []byte(modified), 0644); err != nil {
		t.Fatal(err)
	}
	snapshot2, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument (2): %v", err)
	}

	if snapshot1.Fingerprint == snapshot2.Fingerprint {
		t.Error("fingerprint should differ after file modification")
	}
}

func TestLoadDocument_nonexistentFile(t *testing.T) {
	_, err := LoadDocument("/nonexistent/path/agent.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadDocument_sectionPermissionsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(sampleAgentYAML), 0644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}

	// Decode the permissions section back into YAML bytes and verify
	// the structure matches the original permissions content.
	permNode, ok := snapshot.Document.Section("permissions")
	if !ok {
		t.Fatal("expected 'permissions' section")
	}

	// Marshal the section node back to YAML to verify round-trip
	roundTrip, err := yaml.Marshal(permNode)
	if err != nil {
		t.Fatalf("marshal permissions node: %v", err)
	}

	// Verify the round-tripped YAML contains the key fields from the original
	if !strings.Contains(string(roundTrip), "filesystem:") {
		t.Error("round-tripped permissions missing 'filesystem' key")
	}
	if !strings.Contains(string(roundTrip), "/workspace/**") {
		t.Error("round-tripped permissions missing path pattern")
	}
}

func TestLoadDocument_emptySpec(t *testing.T) {
	yamlContent := `apiVersion: relurpify.io/v1
kind: AgentManifest
metadata:
  name: empty-test
spec:
`
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}

	// Empty spec should have nil or empty sections
	_, ok := snapshot.Document.Section("permissions")
	if ok {
		t.Error("expected no 'permissions' section for empty spec")
	}
}

func TestDocument_Section_nil(t *testing.T) {
	var d *Document
	_, ok := d.Section("test")
	if ok {
		t.Error("Section on nil Document should return false")
	}
}
