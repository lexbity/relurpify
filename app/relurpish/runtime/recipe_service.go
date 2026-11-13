// Package runtime provides the relurpish application runtime services.
package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// RecipeService provides recipe file operations with lossless editing,
// immutable revision tracking, and safe confined writes (P04).
type RecipeService struct {
	workspaceRoot string
	// recipeRoot is the directory containing .erpe files (typically relurpify_cfg/euclo)
	recipeRoot string
	// recipeWorkspace is the workspace-relative path to recipe directory
	recipeWorkspace string
}

// NewRecipeService creates a new recipe service for a workspace.
func NewRecipeService(workspaceRoot string) (*RecipeService, error) {
	if workspaceRoot == "" {
		return nil, errors.New("workspace root is required")
	}

	// Default recipe root is relurpify_cfg/euclo under workspace
	recipeRoot := filepath.Join(workspaceRoot, "relurpify_cfg", "euclo")
	recipeWorkspace := "relurpify_cfg/euclo"

	// Check if the recipe directory exists
	if _, err := os.Stat(recipeRoot); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("check recipe directory: %w", err)
		}
		// Create the directory if it doesn't exist
		if err := os.MkdirAll(recipeRoot, 0755); err != nil {
			return nil, fmt.Errorf("create recipe directory: %w", err)
		}
	}

	return &RecipeService{
		workspaceRoot:  workspaceRoot,
		recipeRoot:     recipeRoot,
		recipeWorkspace: recipeWorkspace,
	}, nil
}

// ListRecipes returns all recipe files in the workspace.
func (s *RecipeService) ListRecipes(ctx context.Context) ([]RecipeFileInfo, error) {
	entries, err := os.ReadDir(s.recipeRoot)
	if err != nil {
		return nil, fmt.Errorf("read recipe directory: %w", err)
	}

	var recipes []RecipeFileInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".erpe") {
			continue
		}

		fullPath := filepath.Join(s.recipeRoot, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Compute digest
		digest, err := s.computeFileDigest(fullPath)
		if err != nil {
			continue
		}

		recipes = append(recipes, RecipeFileInfo{
			ID:       strings.TrimSuffix(entry.Name(), ".erpe"),
			Path:     fullPath,
			WorkspacePath: filepath.Join(s.recipeWorkspace, entry.Name()),
			Revision: digest,
			Size:     info.Size(),
			Modified: info.ModTime(),
		})
	}

	return recipes, nil
}

// OpenRecipe opens a recipe file and returns its content with revision info.
func (s *RecipeService) OpenRecipe(ctx context.Context, path string) (*RecipeFile, error) {
	// Resolve the path - can be workspace-relative or absolute
	absPath, err := s.resolveRecipePath(path)
	if err != nil {
		return nil, err
	}

	// Check if the file exists and is within the recipe root
	if !s.isWithinRecipeRoot(absPath) {
		return nil, fmt.Errorf("path %q is outside recipe directory", path)
	}

	// Read the file
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read recipe file: %w", err)
	}

	// Compute digest
	digest := s.computeDigest(content)

	// Parse to get name
	name, err := s.extractRecipeName(string(content))
	if err != nil {
		name = filepath.Base(absPath)
	}

	return &RecipeFile{
		ID:       strings.TrimSuffix(filepath.Base(absPath), ".erpe"),
		Path:     absPath,
		WorkspacePath: filepath.Join(s.recipeWorkspace, filepath.Base(absPath)),
		Name:     name,
		Source:   string(content),
		Revision: digest,
		Bytes:    content,
	}, nil
}

// SaveRecipe saves a recipe file with atomic write and revision check.
// Returns the saved file info or an error if the revision doesn't match.
func (s *RecipeService) SaveRecipe(ctx context.Context, file *RecipeFile) (*RecipeFile, error) {
	// Validate the path is within the recipe directory
	absPath, err := s.resolveRecipePath(file.Path)
	if err != nil {
		return nil, err
	}

	if !s.isWithinRecipeRoot(absPath) {
		return nil, fmt.Errorf("path %q is outside recipe directory", file.Path)
	}

	// Ensure the path ends with .erpe
	if !strings.HasSuffix(absPath, ".erpe") {
		absPath += ".erpe"
	}

	// If this is an update, check the existing revision
	if file.Revision != "" {
		currentFile, err := s.OpenRecipe(ctx, absPath)
		if err == nil {
			// File exists, check revision
			if currentFile.Revision != file.Revision {
				return nil, fmt.Errorf("revision conflict: expected %s, got %s",
					file.Revision, currentFile.Revision)
			}
		}
	}

	// Compute new digest
	newDigest := s.computeDigest([]byte(file.Source))

	// Validate the recipe syntax
	if _, err := thoughtrecipe.ParseSource(absPath, file.Source); err != nil {
		return nil, fmt.Errorf("parse recipe: %w", err)
	}

	// Perform atomic write
	if err := s.atomicWrite(absPath, []byte(file.Source)); err != nil {
		return nil, fmt.Errorf("write recipe: %w", err)
	}

	return &RecipeFile{
		ID:       strings.TrimSuffix(filepath.Base(absPath), ".erpe"),
		Path:     absPath,
		WorkspacePath: filepath.Join(s.recipeWorkspace, filepath.Base(absPath)),
		Name:     file.Name,
		Source:   file.Source,
		Revision: newDigest,
		Bytes:    []byte(file.Source),
	}, nil
}

// PrepareRecipeRun validates and prepares a recipe for execution.
// Returns a RecipeRunReview that can be confirmed before actual execution.
func (s *RecipeService) PrepareRecipeRun(ctx context.Context, file *RecipeFile) (*RecipeRunReview, error) {
	// Validate the recipe
	doc, err := thoughtrecipe.ParseSource(file.Path, file.Source)
	if err != nil {
		return nil, fmt.Errorf("parse recipe: %w", err)
	}

	// Create manifest with immutable closure
	manifest := &RecipeManifest{
		Root: RecipeRevision{
			Path:   file.WorkspacePath,
			Digest: file.Revision,
		},
		Entry: doc.Name,
	}

	// For now, we'll just return a basic review
	// In a full implementation, we'd resolve imports and validate the closure
	return &RecipeRunReview{
		Manifest:     manifest,
		RecipeName:   doc.Name,
		RecipePath:   file.WorkspacePath,
		RecipeDigest: file.Revision,
		Entry:        doc.Name,
		Valid:        true,
		Issues:       nil,
	}, nil
}

// resolveRecipePath resolves a recipe path to an absolute path.
func (s *RecipeService) resolveRecipePath(path string) (string, error) {
	// If absolute, use as-is
	if filepath.IsAbs(path) {
		return path, nil
	}

	// If workspace-relative, join with workspace root
	if !filepath.IsAbs(path) {
		return filepath.Join(s.workspaceRoot, path), nil
	}

	return path, nil
}

// isWithinRecipeRoot checks if a path is within the recipe directory.
func (s *RecipeService) isWithinRecipeRoot(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRecipeRoot, _ := filepath.Abs(s.recipeRoot)
	return strings.HasPrefix(absPath, absRecipeRoot+string(filepath.Separator)) ||
		absPath == absRecipeRoot
}

// computeFileDigest computes the SHA-256 digest of a file.
func (s *RecipeService) computeFileDigest(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return s.computeDigest(content), nil
}

// computeDigest computes the SHA-256 digest of byte content.
func (s *RecipeService) computeDigest(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// extractRecipeName extracts the recipe name from the first line.
func (s *RecipeService) extractRecipeName(content string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return "", errors.New("empty file")
	}
	line := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(line, "thoughtrecipe") {
		return "", errors.New("not a recipe file")
	}
	// Extract the name after "thoughtrecipe"
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", errors.New("missing recipe name")
	}
	return parts[1], nil
}

// atomicWrite performs an atomic write to a file.
func (s *RecipeService) atomicWrite(path string, content []byte) error {
	// Create temp file in same directory
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	// Write content
	if _, err := tmpFile.Write(content); err != nil {
		return err
	}

	// Sync to disk
	if err := tmpFile.Sync(); err != nil {
		return err
	}

	// Close before rename
	if err := tmpFile.Close(); err != nil {
		return err
	}

	// Atomic rename
	return os.Rename(tmpFile.Name(), path)
}

// RecipeFile represents a recipe file with its metadata.
type RecipeFile struct {
	// ID is the recipe identifier (filename without extension).
	ID string
	// Path is the absolute file path.
	Path string
	// WorkspacePath is the workspace-relative path.
	WorkspacePath string
	// Name is the recipe name from the file content.
	Name string
	// Source is the recipe source text.
	Source string
	// Revision is the content digest (SHA-256).
	Revision string
	// Bytes is the raw file content.
	Bytes []byte
}

// RecipeFileInfo is a lightweight info about a recipe file.
type RecipeFileInfo struct {
	ID           string
	Path         string
	WorkspacePath string
	Revision     string
	Size         int64
	Modified     interface{} // time.Time
}

// RecipeRevision uniquely identifies a recipe file state.
type RecipeRevision struct {
	Path   string
	Digest string
}

// RecipeManifest captures the complete immutable closure for a recipe.
type RecipeManifest struct {
	// Root is the main recipe being executed.
	Root RecipeRevision
	// Imports is the set of imported recipes with their digests.
	Imports map[string]RecipeRevision
	// Entry is the entry point name.
	Entry string
}

// RecipeRunReview is the result of validating a recipe for execution.
type RecipeRunReview struct {
	// Manifest is the complete recipe closure.
	Manifest *RecipeManifest
	// RecipeName is the recipe's declared name.
	RecipeName string
	// RecipePath is the workspace-relative path.
	RecipePath string
	// RecipeDigest is the SHA-256 hash of the root recipe.
	RecipeDigest string
	// Entry is the entry point.
	Entry string
	// Valid indicates whether the recipe is valid for execution.
	Valid bool
	// Issues contains any validation issues.
	Issues []string
}

// WithConfig creates a recipe service using the provided config.
// Extracts the workspace root from the document source path.
func NewRecipeServiceFromConfig(cfg *config.DocumentSnapshot) (*RecipeService, error) {
	if cfg == nil || cfg.SourcePath == "" {
		return nil, errors.New("config must have a valid source path")
	}
	return NewRecipeService(filepath.Dir(cfg.SourcePath))
}

// NewRecipeServiceFromRuntimeConfig creates a recipe service from runtime config.
func NewRecipeServiceFromRuntimeConfig(workspaceRoot string) (*RecipeService, error) {
	return NewRecipeService(workspaceRoot)
}

// WorkspaceRelativePath converts an absolute path to a workspace-relative path.
func (s *RecipeService) WorkspaceRelativePath(absPath string) string {
	rel, err := filepath.Rel(s.workspaceRoot, absPath)
	if err != nil {
		return absPath
	}
	return rel
}
