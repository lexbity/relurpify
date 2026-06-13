package modelselect

import (
	"fmt"

	cfgmodel "codeburg.org/lexbit/relurpify/userconfig/config/model"
)

// strictDecode is a Decoder that wraps config.StrictDecode.
// It is set by the model package during init.
var strictDecode cfgmodel.Decoder

// LoadProfileRegistry loads model profiles from config files and builds
// a ProfileRegistry.
func LoadProfileRegistry(configDir string) (*ProfileRegistry, error) {
	loaded, err := cfgmodel.LoadProfileDir(configDir, strictDecode)
	if err != nil {
		return nil, fmt.Errorf("load profiles: %w", err)
	}
	return BuildProfileRegistry(loaded)
}

// BuildProfileRegistry converts DTO configs into domain ModelProfile objects.
func BuildProfileRegistry(configs []*cfgmodel.ModelProfileConfig) (*ProfileRegistry, error) {
	reg := NewProfileRegistry()
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		reg.Add(cfg)
	}
	return reg, nil
}

// LoadProviderRegistry loads provider configs and builds a ProviderRegistry.
func LoadProviderRegistry(dir string) (*ProviderRegistry, error) {
	providers, err := cfgmodel.LoadProviderDir(dir, strictDecode)
	if err != nil {
		return nil, fmt.Errorf("load providers: %w", err)
	}
	return NewProviderRegistry(providers), nil
}
