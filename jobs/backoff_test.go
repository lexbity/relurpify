package jobs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNextBackoffNeverWraps proves D7's overflow clamp: a base near or above
// the cap is clamped BEFORE the shift, so no attempt number can produce a
// negative (wrapped) delay — a negative delay would land NextAttemptAt in
// the past and hot-retry.
func TestNextBackoffNeverWraps(t *testing.T) {
	cases := []struct {
		name  string
		base  time.Duration
		shift int
	}{
		{"max_int64_base", time.Duration(1<<63 - 1), 1},
		{"one_year_base", 365 * 24 * time.Hour, 1},
		{"over_cap_base", 6 * time.Minute, 5},
		{"exactly_cap_base", 5 * time.Minute, 5},
		{"large_base_deep_shift", time.Hour, 32},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NextBackoff(Spec{Backoff: tc.base}, tc.shift)
			require.GreaterOrEqual(t, d, time.Duration(0), "the delay must never wrap negative")
			require.LessOrEqual(t, d, 5*time.Minute, "the delay is capped")
		})
	}
}

// TestNextBackoffExponential proves the ordinary curve is unchanged: base 30s
// doubles per attempt up to the 5-minute cap.
func TestNextBackoffExponential(t *testing.T) {
	spec := Spec{}
	require.Equal(t, 30*time.Second, NextBackoff(spec, 1))
	require.Equal(t, time.Minute, NextBackoff(spec, 2))
	require.Equal(t, 2*time.Minute, NextBackoff(spec, 3))
	require.Equal(t, 4*time.Minute, NextBackoff(spec, 4))
	require.Equal(t, 5*time.Minute, NextBackoff(spec, 5), "capped at 5 minutes")
	require.Equal(t, 5*time.Minute, NextBackoff(spec, 99))
}

// TestSpecValidRejectsAbsurdBackoff proves the submit-time bound (D7): a
// backoff over 24h is a spec error, rejected at Create, not a runtime
// surprise.
func TestSpecValidRejectsAbsurdBackoff(t *testing.T) {
	valid := Spec{Kind: "sweep", Payload: map[string]any{}, Queue: "knowledge"}
	require.NoError(t, valid.Valid())
	require.NoError(t, func() error {
		s := valid
		s.Backoff = 24 * time.Hour
		return s.Valid()
	}())
	over := valid
	over.Backoff = 24*time.Hour + time.Nanosecond
	require.Error(t, over.Valid(), "a backoff over 24h is rejected at submission")
}
