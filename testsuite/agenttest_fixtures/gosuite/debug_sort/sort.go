package debug_sort

import "sort"

// Sort returns a sorted copy of nums in ascending order.
func Sort(nums []int) []int {
	out := append([]int(nil), nums...)
	sort.Ints(out)
	return out
}
