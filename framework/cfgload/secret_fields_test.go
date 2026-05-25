package cfgload

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRejectForbiddenSecretFields(t *testing.T) {
	data := []byte(`model:
  api_key: secret
  nested:
    token: other
`)
	err := RejectForbiddenSecretFields("relurpify_cfg/config.yaml", data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "file=relurpify_cfg/config.yaml")
	require.Contains(t, err.Error(), "field=model.api_key")
}
