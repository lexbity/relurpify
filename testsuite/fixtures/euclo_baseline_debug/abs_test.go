//go:build live
// +build live

package euclobaselinedebug

import "testing"

func TestAbs(t *testing.T) {
	if Abs(-3) != 3 {
		t.Fatalf("got %d, want 3", Abs(-3))
	}
}
