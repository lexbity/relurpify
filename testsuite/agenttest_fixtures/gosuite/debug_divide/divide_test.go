package debug_divide

import "testing"

func TestDivide(t *testing.T) {
	if got := Divide(8, 2); got != 4 {
		t.Fatalf("Divide(8,2) = %v, want 4", got)
	}
}

func TestDivideByZero(t *testing.T) {
	if got := Divide(8, 0); got != 0 {
		t.Fatalf("Divide(8,0) = %v, want 0", got)
	}
}
