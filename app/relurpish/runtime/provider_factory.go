package runtime

import capability "codeburg.org/lexbit/relurpify/capability"

func providerFromConfig(config capability.ProviderConfig) (RuntimeProvider, error) {
	_ = config
	return nil, nil
}
