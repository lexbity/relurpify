package model_test

import (
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
)

func init() {
	model.DecodeWithSchema = func(path string, data []byte, out any) (any, error) {
		return cfgload.DecodeWithSchema(path, data, cfgload.NewSchemaRegistry(), out)
	}
}
