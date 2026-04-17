package greet

import "testing"

func TestGreet(t *testing.T) {
	if got := Greet("Ada"); got != "Hi, Ada" {
		t.Fatalf("Greet(Ada) = %q, want %q", got, "Hi, Ada")
	}
}
