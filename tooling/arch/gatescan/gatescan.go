// Package gatescan walks a repository's Go sources and parses them for the
// AST-based architecture gates.
//
// The grep-based gates these gates replace can be bypassed by constructing the
// forbidden text at runtime (string concatenation, import aliases, symbol
// renames). Inspecting the parsed syntax tree cannot: an aliased import still
// resolves to the package it names, and a symbol is its identifier whatever
// surrounds it.
//
// Known limits, by design: a string assembled by concatenation is a binary
// expression, not a literal, so shimcheck does not see it; that is what the
// grep gates in the Makefile cover as a secondary layer. Reflection-based
// dispatch is equally invisible to both layers.
package gatescan

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultSkipDirs are directory names never analyzed at any depth: caches,
// checkouts, and build output.
var DefaultSkipDirs = []string{".git", ".gomodcache", ".gocache", "dist", "vendor"} //nolint:gochecknoglobals // immutable skip-directory vocabulary

// File is one parsed source file. Path is slash-separated and relative to the
// walk root so violations read the same on every platform.
type File struct {
	Path string
	Fset *token.FileSet
	AST  *ast.File
}

// Options configures a walk.
type Options struct {
	// Root is the directory to walk. Usually the repository root.
	Root string
	// IncludeTests keeps _test.go files in the analysis. Gates that must police
	// production code only leave it false.
	IncludeTests bool
	// SkipDirs adds directory names to DefaultSkipDirs.
	SkipDirs []string
}

// Walk parses every eligible .go file under opts.Root.
//
// Files that fail to parse are reported in ParseErrors instead of aborting the
// walk: deliberately broken fixtures (testdata) must not stop a gate, and the
// caller decides whether a parse failure is fatal. Directories named in
// SkipDirs and .gomodcache/.gocache/dist/vendor are never entered.
func Walk(opts Options) (files []File, parseErrors []string, err error) {
	skip := map[string]bool{}
	for _, d := range DefaultSkipDirs {
		skip[d] = true
	}
	for _, d := range opts.SkipDirs {
		skip[d] = true
	}

	root := opts.Root
	if root == "" {
		root = "."
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (skip[name] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		if !opts.IncludeTests && strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("%s: %v", filepath.ToSlash(rel), parseErr))
			return nil
		}
		files = append(files, File{Path: filepath.ToSlash(rel), Fset: fset, AST: f})
		return nil
	})
	if err != nil {
		return nil, parseErrors, fmt.Errorf("walk %s: %w", root, err)
	}
	return files, parseErrors, nil
}

// ParseSource parses one source file from memory. It is the fixture path for
// gate tests: a violation is planted as a string and inspected without writing
// to disk, so a test states exactly what the gate must catch.
func ParseSource(path, src string) (File, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return File{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return File{Path: filepath.ToSlash(path), Fset: fset, AST: f}, nil
}

// Position renders "path:line" for a node in f.
func Position(f File, node ast.Node) string {
	return fmt.Sprintf("%s:%d", f.Path, f.Fset.Position(node.Pos()).Line)
}

// ImportBindings describes how a file refers to imported packages.
type ImportBindings struct {
	// byName maps a local identifier to the import path it names: "os" -> "os"
	// for a plain import, "o" -> "os" for `import o "os"`.
	byName map[string]string
	// dotted holds import paths brought in unqualified by `import . "path"`.
	dotted map[string]bool
}

// ImportsOf analyzes f's imports so selector expressions can be resolved back
// to the package they name.
func ImportsOf(f *ast.File) ImportBindings {
	b := ImportBindings{byName: map[string]string{}, dotted: map[string]bool{}}
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		switch {
		case imp.Name != nil && imp.Name.Name == ".":
			b.dotted[path] = true
		case imp.Name != nil:
			b.byName[imp.Name.Name] = path
		default:
			b.byName[filepath.Base(path)] = path
		}
	}
	return b
}

// Resolves reports whether ident names importPath inside this file, either
// directly (`os.Getenv`) or through a dot import (bare `Getenv`).
func (b ImportBindings) Resolves(ident *ast.Ident, importPath string) bool {
	if ident == nil {
		return false
	}
	if path, ok := b.byName[ident.Name]; ok {
		return path == importPath
	}
	return b.dotted[importPath]
}

// IsDotImported reports whether importPath was brought in unqualified by an
// `import . "path"` clause, making its members callable as bare identifiers.
func (b ImportBindings) IsDotImported(importPath string) bool {
	return b.dotted[importPath]
}

// LiteralValue unquotes a BasicLit string, falling back to the raw text when
// the literal is not well-formed.
func LiteralValue(lit *ast.BasicLit) string {
	if lit == nil || lit.Kind != token.STRING {
		return ""
	}
	if v, err := strconv.Unquote(lit.Value); err == nil {
		return v
	}
	return lit.Value
}

// HasPathPrefix reports whether path is one of the slash-separated prefixes or
// nested within one, comparing whole components. "userconfig" matches
// "userconfig/config/x.go" but not "userconfigx/x.go".
func HasPathPrefix(path string, prefixes ...string) bool {
	parts := strings.Split(path, "/")
	for n := range parts {
		prefix := strings.Join(parts[:n+1], "/")
		for _, want := range prefixes {
			if prefix == want {
				return true
			}
		}
	}
	return false
}

// ContainsDir reports whether path has any of dirs as a component, at any
// depth. Use it to exclude generated or fixture trees that sit beside
// production sources.
func ContainsDir(path string, dirs ...string) bool {
	for _, part := range strings.Split(path, "/") {
		for _, dir := range dirs {
			if part == dir {
				return true
			}
		}
	}
	return false
}
