//go:build live
// +build live

package euclobaselinedebug

import "testing"

func TestIncrement(t *testing.T) {
	if Increment(3) != 4 {
		t.Fatalf("got %d, want 4", Increment(3))
	}
}
