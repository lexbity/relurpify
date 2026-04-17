package debug_sort

import "testing"

func TestSort(t *testing.T) {
	in := []int{3, 1, 2}
	got := Sort(in)

	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("Sort(%v) length = %d, want %d", in, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Sort(%v)[%d] = %d, want %d", in, i, got[i], want[i])
		}
	}
}
