package runtime

import capability "codeburg.org/lexbit/relurpify/framework/capability"

func providerFromConfig(config capability.ProviderConfig) (RuntimeProvider, error) {
	_ = config
	return nil, nil
}
