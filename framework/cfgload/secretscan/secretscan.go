// Package secretscan provides the canonical definitions for secret-bearing
// field names and other shared config constants. It exists as a leaf package
// so that both framework/cfgload and framework/cfgload/model can import it
// without creating a circular dependency.
package secretscan

// ForbiddenSecretFieldNames contains the canonical denylist of YAML field
// names that are forbidden from appearing in config files. These fields
// carry secret material (API keys, tokens, passwords) that must only
// exist in environment variables.
var ForbiddenSecretFieldNames = map[string]struct{}{
	"apikey":     {},
	"apisecret":  {},
	"credential": {},
	"passwd":     {},
	"password":   {},
	"privatekey": {},
	"secret":     {},
	"token":      {},
}

// RuntimeStateDirName is the canonical name for the runtime state directory.
const RuntimeStateDirName = ".relurpify_state"
