package modelselect

import cfgmodel "codeburg.org/lexbit/relurpify/userconfig/config/model"

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
