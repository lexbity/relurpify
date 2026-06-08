package runtime

import "codeburg.org/lexbit/relurpify/capability/provider"

func providerFromConfig(config provider.ProviderConfig) (RuntimeProvider, error) {
	_ = config
	return nil, nil
}
