// Package thoughtrecipe implements the Euclo thoughtrecipe DSL.
package thoughtrecipe

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// RecipeRevision uniquely identifies an immutable recipe file state.
// It combines the file path with a content digest to ensure that
// any change to the file content creates a new revision.
type RecipeRevision struct {
	// Path is the workspace-relative or absolute file path.
	Path string
	// Digest is the SHA-256 hash of the file's exact bytes.
	Digest string
	// Source is the original source buffer (optional, for in-memory operations).
	Source *SourceBuffer
}

// NewRecipeRevision creates a new revision from a path and digest.
func NewRecipeRevision(path, digest string) RecipeRevision {
	return RecipeRevision{
		Path:   path,
		Digest: digest,
	}
}

// NewRecipeRevisionFromSource creates a revision from a source buffer.
func NewRecipeRevisionFromSource(buffer *SourceBuffer) RecipeRevision {
	return RecipeRevision{
		Path:   buffer.Filename(),
		Digest: buffer.Digest(),
		Source: buffer,
	}
}

// NewRecipeRevisionFromDocumentSource creates a revision from a document source.
func NewRecipeRevisionFromDocumentSource(ds *DocumentSource) RecipeRevision {
	return RecipeRevision{
		Path:   ds.Buffer.Filename(),
		Digest: ds.Digest,
		Source: ds.Buffer,
	}
}

// String returns a human-readable representation.
func (r RecipeRevision) String() string {
	base := filepath.Base(r.Path)
	if len(r.Digest) >= 8 {
		return fmt.Sprintf("%s@%s", base, r.Digest[:8])
	}
	return fmt.Sprintf("%s@%s", base, r.Digest)
}

// IsZero reports whether the revision is uninitialized.
func (r RecipeRevision) IsZero() bool {
	return r.Path == "" && r.Digest == ""
}

// Equal reports whether two revisions are identical.
func (r RecipeRevision) Equal(other RecipeRevision) bool {
	return r.Path == other.Path && r.Digest == other.Digest
}

// RecipeManifest captures the complete immutable closure for a recipe.
// This includes the root recipe and all its transitive imports with their
// exact byte digests (P04/D-09).
type RecipeManifest struct {
	// Root is the main recipe being executed.
	Root RecipeRevision
	// Imports is the set of imported recipes and prompts with their digests.
	// The map key is the import path (as declared in the recipe).
	Imports map[string]RecipeRevision
	// Entry is the entry point name (if specified).
	Entry string
	// ParserVersion is the version of the parser used.
	ParserVersion string
	// LoweringVersion is the version of the lowering pass.
	LoweringVersion string
}

// NewRecipeManifest creates a new manifest from a root revision.
func NewRecipeManifest(root RecipeRevision) *RecipeManifest {
	return &RecipeManifest{
		Root:            root,
		Imports:         make(map[string]RecipeRevision),
		ParserVersion:   "1.0",
		LoweringVersion: "1.0",
	}
}

// AddImport adds an imported recipe to the manifest.
func (m *RecipeManifest) AddImport(path string, rev RecipeRevision) {
	if m.Imports == nil {
		m.Imports = make(map[string]RecipeRevision)
	}
	m.Imports[path] = rev
}

// Digest returns a combined digest of all components in the manifest.
// This uniquely identifies the complete closure.
func (m *RecipeManifest) Digest() string {
	h := sha256.New()
	write := func(value string) {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	write(m.Root.Digest)
	paths := make([]string, 0, len(m.Imports))
	for path := range m.Imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		write(path)
		write(m.Imports[path].Digest)
	}
	write(m.Entry)
	write(m.ParserVersion)
	write(m.LoweringVersion)
	return hex.EncodeToString(h.Sum(nil))
}

// Validate checks that all imports in the manifest have valid digests.
func (m *RecipeManifest) Validate() error {
	if m.Root.IsZero() {
		return fmt.Errorf("root recipe revision is zero")
	}
	for path, rev := range m.Imports {
		if rev.IsZero() {
			return fmt.Errorf("import %q has zero revision", path)
		}
	}
	return nil
}

// ResolutionContext holds the context for resolving a recipe's imports.
type ResolutionContext struct {
	// WorkspaceRoot is the root directory of the workspace.
	WorkspaceRoot string
	// RecipeRoot is the directory containing recipe files (typically relurpify_cfg/euclo).
	RecipeRoot string
	// CurrentFile is the file currently being processed.
	CurrentFile string
}

// ResolveImportPath resolves an import path to an absolute file path.
// This implements the import resolution semantics for Euclo recipes.
func (ctx *ResolutionContext) ResolveImportPath(importPath string) string {
	// Handle absolute paths
	if filepath.IsAbs(importPath) {
		return importPath
	}
	// Handle workspace-relative paths
	if strings.HasPrefix(importPath, "**/") || strings.HasPrefix(importPath, "*") {
		// Glob patterns resolve relative to the recipe root.
		return filepath.Join(ctx.RecipeRoot, importPath)
	}
	// Relative to current file's directory
	return filepath.Join(filepath.Dir(ctx.CurrentFile), importPath)
}

// WorkspaceRelativePath converts an absolute path to a workspace-relative path.
func WorkspaceRelativePath(workspaceRoot, absPath string) string {
	rel, err := filepath.Rel(workspaceRoot, absPath)
	if err != nil {
		return absPath
	}
	return rel
}

// RecipeIdentity combines a name and revision for unique identification.
type RecipeIdentity struct {
	Name     string
	Revision RecipeRevision
}

// NewRecipeIdentity creates a new recipe identity.
func NewRecipeIdentity(name string, rev RecipeRevision) RecipeIdentity {
	return RecipeIdentity{Name: name, Revision: rev}
}

// String returns a human-readable representation.
func (r RecipeIdentity) String() string {
	return fmt.Sprintf("%s@%s", r.Name, r.Revision.String())
}

// Equal reports whether two identities are the same.
func (r RecipeIdentity) Equal(other RecipeIdentity) bool {
	return r.Name == other.Name && r.Revision.Equal(other.Revision)
}
