//go:build !race

package jobsstore

// raceEnabled is true when the test binary runs under the race detector.
// See race_on.go.
const raceEnabled = false
