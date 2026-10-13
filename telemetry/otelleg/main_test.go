package otelleg

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts the package leaks no goroutines: every provider a test starts
// must be shut down by Chain.Close.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
