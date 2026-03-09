package tools

import "fmt"

// NormalizeStringSlice coerces common decoded tool-call array shapes into a
// Go string slice.
func NormalizeStringSlice(value interface{}) ([]string, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []string:
		return append([]string(nil), typed...), nil
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			out = append(out, fmt.Sprint(entry))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected array, got %T", value)
	}
}
