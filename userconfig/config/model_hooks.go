package config

import (
	"codeburg.org/lexbit/relurpify/userconfig/config/model"
)

func init() {
	model.ReadConfigFile = ReadConfigFile
	model.RejectForbiddenSecretFields = RejectForbiddenSecretFields
}
