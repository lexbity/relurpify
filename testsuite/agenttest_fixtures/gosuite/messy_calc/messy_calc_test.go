package messy_calc

import "testing"

func TestDivide(t *testing.T) {
	if got := Divide(9, 3); got != 3 {
		t.Fatalf("Divide(9,3) = %v, want 3", got)
	}
}

func TestDivideByZero(t *testing.T) {
	if got := Divide(9, 0); got != 0 {
		t.Fatalf("Divide(9,0) = %v, want 0", got)
	}
}
