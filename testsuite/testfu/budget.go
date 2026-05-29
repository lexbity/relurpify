package testfu

import "time"

// BudgetedTimeout distributes the remaining time budget evenly across pending
// suites. Each suite gets at least minFloor so no suite is starved.
// When pending is zero or remaining is non-positive, minFloor is returned.
func BudgetedTimeout(remaining time.Duration, pending int, minFloor time.Duration) time.Duration {
	if pending <= 0 || remaining <= 0 {
		return minFloor
	}
	per := remaining / time.Duration(pending)
	if per < minFloor {
		return minFloor
	}
	return per
}
