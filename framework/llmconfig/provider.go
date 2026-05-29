package llmconfig

import (
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	cfgmodel "codeburg.org/lexbit/relurpify/framework/cfgload/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// LoadProviderRegistry loads model provider files from a directory and builds a
// platform/llm provider registry.
func LoadProviderRegistry(dir string) (*llm.ProviderRegistry, error) {
	loaded, err := cfgmodel.LoadProviderDir(dir, cfgload.StrictDecode)
	if err != nil {
		return nil, err
	}
	defs := make([]llm.ProviderDefinition, 0, len(loaded))
	for _, provider := range loaded {
		if provider == nil {
			continue
		}
		defs = append(defs, llm.ProviderDefinition{
			Name:                  provider.Name,
			Kind:                  provider.Kind,
			Endpoint:              provider.Endpoint,
			RequestTimeoutSeconds: provider.RequestTimeoutSeconds,
			AvailableModels:       append([]string(nil), provider.AvailableModels...),
			NativeToolCalling:     provider.NativeToolCalling,
			MaxConcurrent:         provider.MaxConcurrent,
			SourcePath:            provider.SourcePath,
		})
	}
	return llm.NewProviderRegistryFromDefinitions(defs)
}
