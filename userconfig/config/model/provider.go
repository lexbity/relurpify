package model

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var providerNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ResolvedProvider is the canonical provider manifest loaded from relurpify_cfg/model/provider.
type ResolvedProvider struct {
	Schema                string   `yaml:"schema"`
	Name                  string   `yaml:"name"`
	Endpoint              string   `yaml:"endpoint"`
	Kind                  string   `yaml:"kind"`
	RequestTimeoutSeconds int      `yaml:"request_timeout_seconds,omitempty"`
	AvailableModels       []string `yaml:"available_models,omitempty"`
	NativeToolCalling     bool     `yaml:"native_tool_calling,omitempty"`
	MaxConcurrent         int      `yaml:"max_concurrent,omitempty"`
	Description           string   `yaml:"description,omitempty"`
	SetupHint             string   `yaml:"setup_hint,omitempty"`
	SourcePath            string   `yaml:"-"`
}

// LoadProviderDir loads all *.provider.yaml files from model/provider/.
// It returns a hard error if a blocking diagnostic is encountered.
func LoadProviderDir(dir string, decode Decoder) ([]*ResolvedProvider, error) {
	loaded, diags, err := LoadProviderDirDetailed(dir, decode)
	if err != nil {
		return nil, err
	}
	if HasBlockingDiagnostics(diags) {
		return nil, diagnosticsError("provider", diags)
	}
	return loaded, nil
}

// LoadProviderDirDetailed loads provider manifests and preserves partial
// results together with per-file diagnostics.
func LoadProviderDirDetailed(dir string, decode Decoder) ([]*ResolvedProvider, []LoadDiagnostic, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil, fmt.Errorf("provider dir required")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve provider dir: %w", err)
	}
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, []LoadDiagnostic{{Path: absDir, Severity: "blocking", Message: fmt.Sprintf("read provider dir: %v", err)}}, nil
	}
	if len(entries) == 0 {
		return nil, []LoadDiagnostic{{Path: absDir, Severity: "blocking", Message: fmt.Sprintf("provider dir %q is empty", absDir)}}, nil
	}

	workspaceRoot := filepath.Clean(filepath.Join(absDir, "..", "..", ".."))
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".provider.yaml") {
			continue
		}
		paths = append(paths, filepath.Join(absDir, name))
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, []LoadDiagnostic{{Path: absDir, Severity: "blocking", Message: fmt.Sprintf("provider dir %q contains no *.provider.yaml files", absDir)}}, nil
	}

	var diags []LoadDiagnostic
	loaded := make([]*ResolvedProvider, 0, len(paths))
	seenNames := map[string]string{}
	for _, path := range paths {
		body, err := readConfigFile(workspaceRoot, path)
		if err != nil {
			diags = append(diags, LoadDiagnostic{Path: path, Severity: "blocking", Message: fmt.Sprintf("read %s: %v", path, err)})
			continue
		}
		if err := rejectForbiddenSecretFields(path, body); err != nil {
			diags = append(diags, LoadDiagnostic{Path: path, Severity: "blocking", Message: err.Error()})
			continue
		}
		if decode == nil {
			diags = append(diags, LoadDiagnostic{Path: path, Severity: "blocking", Message: fmt.Sprintf("decoder required for %s", path)})
			continue
		}
		var provider ResolvedProvider
		if _, err := decode(path, body, &provider); err != nil {
			diags = append(diags, LoadDiagnostic{Path: path, Severity: "blocking", Message: err.Error()})
			continue
		}
		provider.SourcePath = path
		if err := validateResolvedProvider(&provider); err != nil {
			diags = append(diags, LoadDiagnostic{Path: path, Severity: "blocking", Message: fmt.Sprintf("%s: %v", path, err)})
			continue
		}
		key := strings.ToLower(strings.TrimSpace(provider.Name))
		if prev, ok := seenNames[key]; ok {
			diags = append(diags, LoadDiagnostic{Path: path, Severity: "blocking", Message: fmt.Sprintf("duplicate provider name %q in %s and %s", provider.Name, prev, path)})
			continue
		}
		seenNames[key] = path
		loaded = append(loaded, &provider)
	}

	return loaded, diags, nil
}

func validateResolvedProvider(provider *ResolvedProvider) error {
	if provider == nil {
		return fmt.Errorf("provider required")
	}
	provider.Name = strings.TrimSpace(provider.Name)
	provider.Endpoint = strings.TrimSpace(provider.Endpoint)
	provider.Kind = strings.ToLower(strings.TrimSpace(provider.Kind))
	if provider.Name == "" {
		return fmt.Errorf("name required")
	}
	if !providerNamePattern.MatchString(provider.Name) {
		return fmt.Errorf("name %q must match [A-Za-z0-9_-]+", provider.Name)
	}
	if err := validateProviderKind(provider.Kind); err != nil {
		return err
	}
	if _, err := url.ParseRequestURI(provider.Endpoint); err != nil {
		return fmt.Errorf("endpoint invalid: %w", err)
	}
	if !strings.HasPrefix(provider.Endpoint, "http://") && !strings.HasPrefix(provider.Endpoint, "https://") {
		return fmt.Errorf("endpoint must use http or https: %q", provider.Endpoint)
	}
	provider.AvailableModels = normalizeStrings(provider.AvailableModels)
	if provider.RequestTimeoutSeconds < 0 {
		return fmt.Errorf("request_timeout_seconds must be >= 0")
	}
	if provider.MaxConcurrent < 0 {
		return fmt.Errorf("max_concurrent must be >= 0")
	}
	return nil
}

func validateProviderKind(kind string) error {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "ollama", "openai_compatible", "lmstudio", "offline":
		return nil
	default:
		return fmt.Errorf("kind %q must be one of ollama, openai_compatible, lmstudio, offline", kind)
	}
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
