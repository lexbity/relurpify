package debug_panic_handler

import "testing"

func TestProcessItems(t *testing.T) {
	if got := ProcessItems([]string{"a", "b"}); got != "a" {
		t.Fatalf("ProcessItems([a b]) = %q, want %q", got, "a")
	}
}

func TestProcessItemsEmpty(t *testing.T) {
	if got := ProcessItems(nil); got != "" {
		t.Fatalf("ProcessItems(nil) = %q, want empty string", got)
	}
}
