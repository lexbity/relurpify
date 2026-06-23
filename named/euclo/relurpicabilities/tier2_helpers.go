package relurpicabilities

import (
	"bytes"
	goast "go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	frameworkast "codeburg.org/lexbit/relurpify/context/knowledge/ast"
)

// resolveCandidatePath resolves a relative or absolute path against a workspace
// root. Returns empty string on empty input.
func resolveCandidatePath(candidate, workspace string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	if filepath.IsAbs(candidate) {
		return filepath.Clean(candidate)
	}
	return filepath.Clean(filepath.Join(workspace, candidate))
}
func floatArg(args map[string]any, key string, defaultValue float64) (float64, bool) {
	val, ok := args[key]
	if !ok || val == nil {
		return defaultValue, false
	}
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return parsed, true
		}
	}
	return defaultValue, false
}

func parseCoverageOutput(output string) (map[string]float64, []coveragePackageRecord) {
	coverage := make(map[string]float64)
	records := make([]coveragePackageRecord, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "ok" {
			continue
		}
		pkg := fields[1]
		idx := strings.Index(line, "coverage:")
		if idx < 0 {
			continue
		}
		segment := strings.TrimSpace(line[idx+len("coverage:"):])
		segment = strings.TrimSpace(strings.TrimSuffix(segment, "of statements"))
		segment = strings.TrimSpace(strings.TrimSuffix(segment, "%"))
		segment = strings.TrimSpace(strings.TrimSuffix(segment, "of statements"))
		segment = strings.TrimSpace(strings.TrimSuffix(segment, "%"))
		percent, err := strconv.ParseFloat(segment, 64)
		if err != nil {
			continue
		}
		coverage[pkg] = percent
		records = append(records, coveragePackageRecord{Package: pkg, Coverage: percent})
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Package < records[j].Package })
	return coverage, records
}

type apiSignatureRecord struct {
	File      string
	Symbol    string
	Signature string
}

type apiChangeRecord struct {
	File          string
	Symbol        string
	BaseSignature string
	HeadSignature string
	Change        string
}

type coveragePackageRecord struct {
	Package  string
	Coverage float64
}

func collectExportedAPISignatures(path string, src []byte) (map[string]apiSignatureRecord, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, err
	}
	out := make(map[string]apiSignatureRecord)
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *goast.FuncDecl:
			if d.Name == nil || !d.Name.IsExported() {
				continue
			}
			symbol := functionSymbolName(d)
			out[signatureKey(path, symbol)] = apiSignatureRecord{
				File:      path,
				Symbol:    symbol,
				Signature: renderFuncSignature(d),
			}
		case *goast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *goast.TypeSpec:
					if s.Name == nil || !s.Name.IsExported() {
						continue
					}
					out[signatureKey(path, s.Name.Name)] = apiSignatureRecord{
						File:      path,
						Symbol:    s.Name.Name,
						Signature: renderTypeSignature(s),
					}
				case *goast.ValueSpec:
					for _, name := range s.Names {
						if name == nil || !name.IsExported() {
							continue
						}
						out[signatureKey(path, name.Name)] = apiSignatureRecord{
							File:      path,
							Symbol:    name.Name,
							Signature: renderValueSignature(strings.ToLower(d.Tok.String()), name.Name, s),
						}
					}
				}
			}
		}
	}
	return out, nil
}

func renderFuncSignature(fn *goast.FuncDecl) string {
	if fn == nil || fn.Name == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("func ")
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		b.WriteString("(")
		b.WriteString(renderFieldListType(fn.Recv))
		b.WriteString(").")
	}
	b.WriteString(fn.Name.Name)
	b.WriteString("(")
	b.WriteString(renderFieldListType(fn.Type.Params))
	b.WriteString(")")
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		b.WriteString(" ")
		b.WriteString(renderResultListType(fn.Type.Results))
	}
	return strings.TrimSpace(b.String())
}

func functionSymbolName(fn *goast.FuncDecl) string {
	if fn == nil || fn.Name == nil {
		return ""
	}
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := strings.TrimSpace(renderFieldListType(fn.Recv))
	recv = strings.TrimPrefix(recv, "*")
	return strings.TrimSpace(recv) + "." + fn.Name.Name
}

func renderTypeSignature(spec *goast.TypeSpec) string {
	if spec == nil || spec.Name == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("type ")
	b.WriteString(spec.Name.Name)
	if spec.Assign.IsValid() {
		b.WriteString(" = ")
	} else {
		b.WriteString(" ")
	}
	b.WriteString(renderExpr(spec.Type))
	return strings.TrimSpace(b.String())
}

func renderValueSignature(kind, name string, spec *goast.ValueSpec) string {
	var b strings.Builder
	b.WriteString(kind)
	b.WriteString(" ")
	b.WriteString(name)
	if spec.Type != nil {
		b.WriteString(" ")
		b.WriteString(renderExpr(spec.Type))
	}
	return strings.TrimSpace(b.String())
}

func renderFieldListType(fields *goast.FieldList) string {
	if fields == nil || len(fields.List) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		if field == nil {
			continue
		}
		typeText := renderExpr(field.Type)
		if typeText == "" {
			continue
		}
		if len(field.Names) > 0 {
			for range field.Names {
				parts = append(parts, typeText)
			}
			continue
		}
		parts = append(parts, typeText)
	}
	return strings.Join(parts, ", ")
}

func renderResultListType(fields *goast.FieldList) string {
	if fields == nil || len(fields.List) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		if field == nil {
			continue
		}
		typeText := renderExpr(field.Type)
		if typeText == "" {
			continue
		}
		if len(field.Names) > 0 {
			for range field.Names {
				parts = append(parts, typeText)
			}
			continue
		}
		parts = append(parts, typeText)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func renderExpr(expr goast.Expr) string {
	if expr == nil {
		return ""
	}
	var b bytes.Buffer
	if err := printer.Fprint(&b, token.NewFileSet(), expr); err != nil {
		return ""
	}
	return strings.TrimSpace(b.String())
}

func signatureKey(file, symbol string) string {
	return strings.TrimSpace(file) + "::" + strings.TrimSpace(symbol)
}

func compareAPISignatures(base, head map[string]apiSignatureRecord) ([]apiChangeRecord, []apiChangeRecord) {
	breaking := make([]apiChangeRecord, 0)
	compatible := make([]apiChangeRecord, 0)
	keys := make([]string, 0, len(base)+len(head))
	for key := range base {
		keys = append(keys, key)
	}
	for key := range head {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	seen := map[string]struct{}{}
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		baseRec, baseOK := base[key]
		headRec, headOK := head[key]
		switch {
		case baseOK && !headOK:
			breaking = append(breaking, apiChangeRecord{File: baseRec.File, Symbol: baseRec.Symbol, BaseSignature: baseRec.Signature, Change: "removed"})
		case !baseOK && headOK:
			compatible = append(compatible, apiChangeRecord{File: headRec.File, Symbol: headRec.Symbol, HeadSignature: headRec.Signature, Change: "added"})
		case baseOK && headOK && baseRec.Signature != headRec.Signature:
			breaking = append(breaking, apiChangeRecord{File: headRec.File, Symbol: headRec.Symbol, BaseSignature: baseRec.Signature, HeadSignature: headRec.Signature, Change: "modified"})
		}
	}
	sort.SliceStable(breaking, func(i, j int) bool {
		if breaking[i].File == breaking[j].File {
			return breaking[i].Symbol < breaking[j].Symbol
		}
		return breaking[i].File < breaking[j].File
	})
	sort.SliceStable(compatible, func(i, j int) bool {
		if compatible[i].File == compatible[j].File {
			return compatible[i].Symbol < compatible[j].Symbol
		}
		return compatible[i].File < compatible[j].File
	})
	return breaking, compatible
}

func changeRecordSlice(records []apiChangeRecord) []any {
	out := make([]any, 0, len(records))
	for _, record := range records {
		out = append(out, map[string]any{
			"file":           record.File,
			"symbol":         record.Symbol,
			"base_signature": record.BaseSignature,
			"head_signature": record.HeadSignature,
			"change":         record.Change,
		})
	}
	return out
}

func coveragePackagesToInterfaces(packages []coveragePackageRecord) []any {
	out := make([]any, 0, len(packages))
	for _, pkg := range packages {
		out = append(out, map[string]any{
			"package":  pkg.Package,
			"coverage": pkg.Coverage,
		})
	}
	return out
}

func nodePathFromStore(store EdgeStore, node *frameworkast.Node, _ string) string {
	if node == nil {
		return ""
	}
	if store == nil {
		return strings.TrimSpace(node.FileID)
	}
	if meta, err := store.GetFile(node.FileID); err == nil && meta != nil && strings.TrimSpace(meta.Path) != "" {
		return meta.Path
	}
	if meta, err := store.GetFileByPath(node.FileID); err == nil && meta != nil && strings.TrimSpace(meta.Path) != "" {
		return meta.Path
	}
	return strings.TrimSpace(node.FileID)
}

func packageLayerForPath(workspace, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	root := strings.TrimSpace(workspace)
	if root != "" {
		if rel, err := filepath.Rel(root, path); err == nil {
			path = rel
		}
	}
	path = filepath.ToSlash(filepath.Clean(path))
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}
