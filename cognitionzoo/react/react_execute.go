package react

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// envGetString is a helper to get a string value from the envelope working memory.
// This replaces the old state.GetString() method.
func envGetString(env *contextdata.Envelope, key string) string {
	if env == nil {
		return ""
	}
	val, ok := env.GetWorkingValue(key)
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(val))
}
