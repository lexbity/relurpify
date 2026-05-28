package cfgload

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"gopkg.in/yaml.v3"
)

// RuntimeCapabilitySelector mirrors the persisted capability selector shape
// without depending on framework/core or agentspec.
type RuntimeCapabilitySelector struct {
	ID                          string                 `yaml:"id,omitempty"`
	Name                        string                 `yaml:"name,omitempty"`
	Kind                        string                 `yaml:"kind,omitempty"`
	RuntimeFamilies             []string               `yaml:"runtime_families,omitempty"`
	Tags                        []string               `yaml:"tags,omitempty"`
	ExcludeTags                 []string               `yaml:"exclude_tags,omitempty"`
	SourceScopes                []string               `yaml:"source_scopes,omitempty"`
	TrustClasses                []string               `yaml:"trust_classes,omitempty"`
	RiskClasses                 []string               `yaml:"risk_classes,omitempty"`
	EffectClasses               []string               `yaml:"effect_classes,omitempty"`
	CoordinationRoles           []string               `yaml:"coordination_roles,omitempty"`
	CoordinationTaskTypes       []string               `yaml:"coordination_task_types,omitempty"`
	CoordinationExecutionModes  []string               `yaml:"coordination_execution_modes,omitempty"`
	CoordinationLongRunning     agentspec.EnabledState `yaml:"coordination_long_running,omitempty"`
	CoordinationDirectInsertion agentspec.EnabledState `yaml:"coordination_direct_insertion,omitempty"`
}

// RuntimeWorkspaceConfig captures persisted workspace preferences under relurpify_cfg.
type RuntimeWorkspaceConfig struct {
	Model               string                        `yaml:"model"`
	Provider            string                        `yaml:"provider,omitempty"`
	SandboxBackend      string                        `yaml:"sandbox_backend,omitempty"`
	Agents              []string                      `yaml:"agents"`
	AllowedCapabilities []RuntimeCapabilitySelector   `yaml:"allowed_capabilities,omitempty"`
	Nexus               RuntimeNexusConfig            `yaml:"nexus,omitempty"`
	NodeRegistration    RuntimeNodeRegistrationConfig `yaml:"node_registration,omitempty"`
	LastUpdated         int64                         `yaml:"last_updated"`
}

// RuntimeProviderConfig captures the persisted provider editor state for relurpish.
type RuntimeProviderConfig struct {
	Provider          string `yaml:"provider"`
	Endpoint          string `yaml:"endpoint,omitempty"`
	Model             string `yaml:"model,omitempty"`
	Timeout           string `yaml:"timeout,omitempty"`
	NativeToolCalling bool   `yaml:"native_tool_calling,omitempty"`
	LastUpdated       int64  `yaml:"last_updated"`
}

// RuntimeKeybindingConfig captures persisted shell and surface keybindings.
type RuntimeKeybindingConfig struct {
	Bindings []RuntimeKeybindingEntry `yaml:"bindings"`
}

// RuntimeKeybindingEntry captures one remappable binding.
type RuntimeKeybindingEntry struct {
	Action      string   `yaml:"action"`
	Keys        []string `yaml:"keys"`
	Scope       string   `yaml:"scope,omitempty"`
	Source      string   `yaml:"source,omitempty"`
	Description string   `yaml:"description,omitempty"`
	DefaultKeys []string `yaml:"default_keys,omitempty"`
}

type RuntimeNexusConfig struct {
	Enabled       bool   `yaml:"enabled,omitempty"`
	Address       string `yaml:"address,omitempty"`
	AutoReconnect bool   `yaml:"auto_reconnect,omitempty"`
}

type RuntimeNodeRegistrationConfig struct {
	Enabled   bool              `yaml:"enabled,omitempty"`
	NodeID    string            `yaml:"node_id,omitempty"`
	Name      string            `yaml:"name,omitempty"`
	Platform  string            `yaml:"platform,omitempty"`
	Tags      map[string]string `yaml:"tags,omitempty"`
	LocalOnly bool              `yaml:"local_only,omitempty"`
}

// LoadRuntimeWorkspaceConfig loads workspace preferences from disk.
func LoadRuntimeWorkspaceConfig(path string) (RuntimeWorkspaceConfig, error) {
	if path == "" {
		return RuntimeWorkspaceConfig{}, fmt.Errorf("config path required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkspaceConfig{}, err
	}
	if err := RejectForbiddenSecretFields(path, data); err != nil {
		return RuntimeWorkspaceConfig{}, err
	}
	var cfg RuntimeWorkspaceConfig
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return RuntimeWorkspaceConfig{}, err
	}
	return cfg, nil
}

// LoadRuntimeProviderConfig loads the persisted provider editor state.
func LoadRuntimeProviderConfig(path string) (RuntimeProviderConfig, error) {
	if path == "" {
		return RuntimeProviderConfig{}, fmt.Errorf("provider config path required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeProviderConfig{}, err
	}
	if err := RejectForbiddenSecretFields(path, data); err != nil {
		return RuntimeProviderConfig{}, err
	}
	var cfg RuntimeProviderConfig
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return RuntimeProviderConfig{}, err
	}
	return cfg, nil
}

// LoadRuntimeKeybindingConfig loads persisted keybinding editor state.
func LoadRuntimeKeybindingConfig(path string) (RuntimeKeybindingConfig, error) {
	if path == "" {
		return RuntimeKeybindingConfig{}, fmt.Errorf("keybinding config path required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeKeybindingConfig{}, err
	}
	if err := RejectForbiddenSecretFields(path, data); err != nil {
		return RuntimeKeybindingConfig{}, err
	}
	var cfg RuntimeKeybindingConfig
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return RuntimeKeybindingConfig{}, err
	}
	return cfg, nil
}

// SaveRuntimeWorkspaceConfig persists selections for future sessions.
func SaveRuntimeWorkspaceConfig(path string, cfg RuntimeWorkspaceConfig) error {
	if path == "" {
		return fmt.Errorf("config path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// SaveRuntimeWorkspaceConfigWithBackup snapshots the existing config before writing.
func SaveRuntimeWorkspaceConfigWithBackup(path string, cfg RuntimeWorkspaceConfig) (string, error) {
	if path == "" {
		return "", fmt.Errorf("config path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	backup, err := CreateTimestampedBackup(path)
	if err != nil {
		return "", err
	}
	if err := SaveRuntimeWorkspaceConfig(path, cfg); err != nil {
		return "", err
	}
	return backup, nil
}

// SaveRuntimeProviderConfig persists provider editor state with a backup snapshot.
func SaveRuntimeProviderConfigWithBackup(path string, cfg RuntimeProviderConfig) (string, error) {
	if path == "" {
		return "", fmt.Errorf("provider config path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	backup, err := CreateTimestampedBackup(path)
	if err != nil {
		return "", err
	}
	if err := SaveYAML(path, cfg); err != nil {
		return "", err
	}
	return backup, nil
}

// SaveRuntimeKeybindingConfigWithBackup persists keybinding editor state with a backup snapshot.
func SaveRuntimeKeybindingConfigWithBackup(path string, cfg RuntimeKeybindingConfig) (string, error) {
	if path == "" {
		return "", fmt.Errorf("keybinding config path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	backup, err := CreateTimestampedBackup(path)
	if err != nil {
		return "", err
	}
	if err := SaveYAML(path, cfg); err != nil {
		return "", err
	}
	return backup, nil
}

// SaveYAML marshals v to YAML and overwrites path.
func SaveYAML(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// SaveJSON marshals v to pretty JSON and overwrites path.
func SaveJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// CreateTimestampedBackup copies path into relurpify_cfg/backups with a timestamped .bak suffix.
func CreateTimestampedBackup(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	backupDir := filepath.Join(filepath.Dir(path), "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	base := filepath.Base(path)
	stamp := time.Now().UTC().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("%s.%s.bak", base, stamp))
	if _, err := os.Stat(backupPath); err == nil {
		for i := 2; ; i++ {
			candidate := filepath.Join(backupDir, fmt.Sprintf("%s.%s.%d.bak", base, stamp, i))
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				backupPath = candidate
				break
			}
		}
	}
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return "", err
	}
	if err := pruneTimestampedBackups(backupDir, base, 10); err != nil {
		return "", err
	}
	return backupPath, nil
}

func pruneTimestampedBackups(backupDir, base string, max int) error {
	if max <= 0 {
		return nil
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return err
	}
	type backupEntry struct {
		path string
		name string
		mod  time.Time
	}
	var backups []backupEntry
	prefix := base + "."
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".bak") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		backups = append(backups, backupEntry{
			path: filepath.Join(backupDir, name),
			name: name,
			mod:  info.ModTime(),
		})
	}
	if len(backups) <= max {
		return nil
	}
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].mod.Equal(backups[j].mod) {
			return backups[i].name < backups[j].name
		}
		return backups[i].mod.Before(backups[j].mod)
	})
	for i := 0; i < len(backups)-max; i++ {
		if err := os.Remove(backups[i].path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
