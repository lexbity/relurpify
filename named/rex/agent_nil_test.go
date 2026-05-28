package rex

import "testing"

func TestNewPanicsOnNilEnvironment(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic")
		}
	}()

	_ = New(nil)
}
