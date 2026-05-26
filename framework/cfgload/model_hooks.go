package cfgload

import (
	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
)

func init() {
	model.ReadConfigFile = ReadConfigFile
	model.RejectForbiddenSecretFields = RejectForbiddenSecretFields
}

