package runtime

import "codeburg.org/lexbit/relurpify/framework/core"

func providerFromConfig(config core.ProviderConfig) (RuntimeProvider, error) {
	_ = config
	return nil, nil
}
