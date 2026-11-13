package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"codeburg.org/lexbit/relurpify/app/relurpish/ports"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// RecipeLibrary reads source through a confined workspace handle. It has no
// save or run methods: live source transactions and captured-file execution
// are not yet integrated, so the frontend must not advertise those actions.
type RecipeLibrary struct{ workspace string }

// NewRecipeLibrary constructs a read service without materializing any files.
func NewRecipeLibrary(workspace string) *RecipeLibrary { return &RecipeLibrary{workspace: workspace} }

// ListRecipes lists a bounded set of regular recipe files without loading bodies.
func (s *RecipeLibrary) ListRecipes(ctx context.Context) (files []ports.RecipeFile, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.workspace)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	dir, err := root.Open("relurpify_cfg/euclo")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, dir.Close()) }()
	entries, err := dir.ReadDir(501)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 500 {
		return nil, fmt.Errorf("recipe library exceeds 500 directory entries; open a source path directly")
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".erpe" {
			continue
		}
		files = append(files, ports.RecipeFile{ID: strings.TrimSuffix(entry.Name(), ".erpe"), Path: "relurpify_cfg/euclo/" + entry.Name()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// OpenRecipe reads bounded UTF-8 source through the confined workspace root.
func (s *RecipeLibrary) OpenRecipe(ctx context.Context, name string) (result ports.RecipeFile, err error) {
	if err := ctx.Err(); err != nil {
		return ports.RecipeFile{}, err
	}
	if !filepath.IsLocal(name) {
		return ports.RecipeFile{}, fmt.Errorf("recipe path must be workspace-relative")
	}
	root, err := os.OpenRoot(s.workspace)
	if err != nil {
		return ports.RecipeFile{}, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	info, err := root.Lstat(name)
	if err != nil {
		return ports.RecipeFile{}, err
	}
	if !info.Mode().IsRegular() {
		return ports.RecipeFile{}, fmt.Errorf("recipe source must be a regular file")
	}
	file, err := root.Open(name)
	if err != nil {
		return ports.RecipeFile{}, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err = file.Stat()
	if err != nil {
		return ports.RecipeFile{}, err
	}
	const sourceLimit = 1 << 20
	if !info.Mode().IsRegular() || info.Size() > sourceLimit {
		return ports.RecipeFile{}, fmt.Errorf("recipe must be a regular file of at most 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, sourceLimit+1))
	if err != nil {
		return ports.RecipeFile{}, err
	}
	if len(data) > sourceLimit || !utf8.Valid(data) {
		return ports.RecipeFile{}, fmt.Errorf("recipe must contain at most 1 MiB of UTF-8 source")
	}
	if err := ctx.Err(); err != nil {
		return ports.RecipeFile{}, err
	}
	digest := sha256.Sum256(data)
	return ports.RecipeFile{ID: strings.TrimSuffix(filepath.Base(name), ".erpe"), Path: filepath.ToSlash(filepath.Clean(name)), Source: string(data), Revision: hex.EncodeToString(digest[:])}, nil
}

// RecipeCatalog projects installed strategy contracts; it claims no prompt-use evidence.
func (s *RecipeLibrary) RecipeCatalog(ctx context.Context) (ports.RecipeCatalog, error) {
	if err := ctx.Err(); err != nil {
		return ports.RecipeCatalog{}, err
	}
	var catalog ports.RecipeCatalog
	for _, contract := range paradigm.Registry.All() {
		catalog.Strategies = append(catalog.Strategies, ports.RecipeStrategy{Name: contract.Paradigm, Shape: string(contract.Shape)})
	}
	sort.Slice(catalog.Strategies, func(i, j int) bool { return catalog.Strategies[i].Name < catalog.Strategies[j].Name })
	return catalog, nil
}
