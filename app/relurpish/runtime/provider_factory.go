package runtime

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/provider"
)

func providerFromConfig(config provider.ProviderConfig) (RuntimeProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%w: id=%s kind=%s", errUnsupportedRuntimeProviderConfig, config.ID, config.Kind)
}
