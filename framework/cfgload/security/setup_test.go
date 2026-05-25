package security_test

import (
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/cfgload/security"
)

func init() {
	security.DecodeWithSchema = func(path string, data []byte, out any) (any, error) {
		return cfgload.DecodeWithSchema(path, data, cfgload.NewSchemaRegistry(), out)
	}
}
