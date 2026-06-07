package config

import (
	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

// InstallSecretFieldChecks wires the shared secret-field validator into the
// sandbox loader. Callers should invoke this during startup before any
// non-cfgload loader is used.
func InstallSecretFieldChecks() {
	sandbox.RejectForbiddenSecretFields = RejectForbiddenSecretFields
}
