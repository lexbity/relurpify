package runtime

import (
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func firstConfigDiagnostic(diags []config.ConfigDiagnostic, section string) *config.ConfigDiagnostic {
	for i := range diags {
		diag := &diags[i]
		if strings.EqualFold(strings.TrimSpace(diag.Section), section) {
			return diag
		}
	}
	return nil
}
