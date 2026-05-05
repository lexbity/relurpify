//go:build live
// +build live

package euclobaselinechat

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatalf("got %d, want 5", Add(2, 3))
	}
}
