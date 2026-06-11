package runtime

import (
	"errors"

	"codeburg.org/lexbit/relurpify/capability/provider"
)

func providerFromConfig(config provider.ProviderConfig) (RuntimeProvider, error) {
	_ = config
	return nil, errors.New("provider from config not implemented")
}
