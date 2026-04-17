package debug_panic_handler

// ProcessItems returns the first item when present.
func ProcessItems(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}
