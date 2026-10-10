//go:build race

package jobsstore

// raceEnabled is true when the test binary runs under the race detector.
// The NFR-3 budget assertion skips itself under -race: the detector's
// overhead dominates the claim path and would false-fail the timing bound.
const raceEnabled = true
