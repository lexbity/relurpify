package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// DocumentMetadata holds identity fields common to all agent config files.
type DocumentMetadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

// Document is a content-agnostic envelope for agent config files. It knows
// that a config file has an apiVersion, kind, metadata, and named spec sections,
// but it does not know what any section means. Each domain owns its section
// decoder (Slice 8+).
type Document struct {
	APIVersion string               `yaml:"apiVersion"`
	Kind       string               `yaml:"kind"`
	Metadata   DocumentMetadata     `yaml:"metadata"`
	Spec       map[string]yaml.Node `yaml:"spec"`
}

// DocumentSnapshot pairs a parsed Document with its source path, raw-file
// fingerprint (SHA-256 of the original bytes), and load metadata.
type DocumentSnapshot struct {
	Document    *Document
	Fingerprint [32]byte
	SourcePath  string
	LoadedAt    time.Time
	Warnings    []string
}

// Section returns the yaml.Node for the given spec section key, or false if
// the section does not exist.
func (d *Document) Section(key string) (yaml.Node, bool) {
	if d == nil || d.Spec == nil {
		return yaml.Node{}, false
	}
	node, ok := d.Spec[key]
	return node, ok
}

// LoadDocument reads an agent config file, decodes it into a content-agnostic
// Document envelope, and fingerprints the raw bytes. It does NOT decode or
// validate any spec section — that is each domain's responsibility.
func LoadDocument(path string) (*DocumentSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read document: %w", err)
	}

	fingerprint := sha256.Sum256(data)

	var doc Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode document envelope: %w", err)
	}

	return &DocumentSnapshot{
		Document:    &doc,
		Fingerprint: fingerprint,
		SourcePath:  path,
		LoadedAt:    time.Now().UTC(),
	}, nil
}

// MustLoadDocument is a test helper that calls LoadDocument and panics on error.
func MustLoadDocument(t interface{ Fatalf(string, ...any) }, path string) *DocumentSnapshot {
	snapshot, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("MustLoadDocument: %v", err)
	}
	return snapshot
}
