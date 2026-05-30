// Command toolgen generates typed parameter structs from tool manifests.
//
// Usage:
//
//	toolgen -manifest <path/to/tool.yaml> -output <path/to/params.gen.go>
//
// The generated file contains a Params struct with exported fields matching
// the manifest's parameters, a ParseParams function that reads from
// map[string]interface{}, and a ParamKeys function returning consumed key names.
package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"gopkg.in/yaml.v3"
)

func main() {
	manifestPath := flag.String("manifest", "", "Path to .tool.yaml manifest")
	outputPath := flag.String("output", "", "Output path for generated .go file (default: same dir as manifest)")
	pkgName := flag.String("pkg", "", "Go package name for generated file (default: derived from manifest family)")
	flag.Parse()

	if *manifestPath == "" {
		fmt.Fprintln(os.Stderr, "error: -manifest flag is required")
		flag.Usage()
		os.Exit(1)
	}

	manifest, err := loadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading manifest %s: %v\n", *manifestPath, err)
		os.Exit(1)
	}

	out := *outputPath
	if out == "" {
		dir := filepath.Dir(*manifestPath)
		name := normalizeFileName(manifest.Name)
		out = filepath.Join(dir, name+"_params.gen.go")
	}

	src, err := generateWithPkg(manifest, *pkgName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error generating code: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(out, src, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", out, err)
		os.Exit(1)
	}
	fmt.Printf("generated %s\n", out)
}

func loadManifest(path string) (*contracts.ToolManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Strip the schema line manually
	lines := bytes.Split(data, []byte("\n"))
	body := data
	for i, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimSuffix(string(line), "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "schema:") {
			body = bytes.Join(lines[i+1:], []byte("\n"))
		}
		break
	}
	var manifest contracts.ToolManifest
	if err := yaml.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("yaml decode: %w", err)
	}
	if manifest.Name == "" {
		return nil, fmt.Errorf("manifest has empty name")
	}
	return &manifest, nil
}

// tmpl is the template for the generated Go file.
//go:embed params.tmpl
var tmplRaw string

type templateData struct {
	PackageName string
	TypePrefix  string // PascalCase prefix for type names (e.g., "SearchGrep" → SearchGrepParams)
	Params      []paramField
	HasParams   bool
}

type paramField struct {
	GoName       string // PascalCase
	YAMLName     string // original name
	GoType       string
	ConvertFn    string // converter function name
	OmitEmpty    bool
	Required     bool
}

func generate(manifest *contracts.ToolManifest) ([]byte, error) {
	return generateWithPkg(manifest, "")
}

func generateWithPkg(manifest *contracts.ToolManifest, pkg string) ([]byte, error) {
	tmpl := template.Must(template.New("params").Parse(tmplRaw))

	fields := make([]paramField, 0, len(manifest.Parameters))
	for _, p := range manifest.Parameters {
		if p.Name == "" {
			continue
		}
		goName := toPascal(p.Name)
		goType := yamlTypeToGo(p.Type)
		fields = append(fields, paramField{
			GoName:    goName,
			YAMLName:  p.Name,
			GoType:    goType,
			ConvertFn: typeToConvertFn(goType),
			OmitEmpty: !p.Required && goType == "string",
			Required:  p.Required,
		})
	}

	if pkg == "" {
		pkg = packageNameForManifest(manifest)
	}
	typePrefix := toPascal(manifest.Name)
	data := templateData{
		PackageName: pkg,
		TypePrefix:  typePrefix,
		Params:      fields,
		HasParams:   len(fields) > 0,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("go fmt: %w\n%s", err, buf.String())
	}
	return src, nil
}

func yamlTypeToGo(t contracts.ToolParameterType) string {
	switch string(t) {
	case "string":
		return "string"
	case "integer", "int":
		return "int64"
	case "number":
		return "float64"
	case "boolean", "bool":
		return "bool"
	case "array":
		return "[]interface{}"
	case "object":
		return "map[string]interface{}"
	default:
		return "string"
	}
}

func typeToConvertFn(goType string) string {
	switch goType {
	case "string":
		return "convString"
	case "int64":
		return "convInt64"
	case "float64":
		return "convFloat64"
	case "bool":
		return "convBool"
	case "[]interface{}":
		return "convSliceInterface"
	case "map[string]interface{}":
		return "convMapStringInterface"
	default:
		return "convString"
	}
}

func toPascal(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	for i, p := range parts {
		parts[i] = upcaseFirst(p)
	}
	return strings.Join(parts, "")
}

func upcaseFirst(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// packageNameForManifest derives a sensible Go package name from the manifest
// family or name. The output directory determines the actual package; this
// is a fallback so the generated code compiles in any package.
func packageNameForManifest(manifest *contracts.ToolManifest) string {
	candidate := manifest.Family
	if candidate == "" {
		candidate = manifest.Name
	}
	candidate = strings.TrimSpace(strings.ToLower(candidate))
	candidate = strings.ReplaceAll(candidate, "-", "")
	candidate = strings.ReplaceAll(candidate, ".", "")
	if candidate == "" {
		return "tool"
	}
	return candidate
}

func normalizeFileName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	return name
}
