package ayenitd

// White-box tests for unexported cron parsing helpers.
// The exported scheduler API is tested in scheduler_export_test.go.

import (
	"testing"
	"time"
)

// --- parseCronField ---

func TestParseCronField_Wildcard(t *testing.T) {
	got, err := parseCronField("*", 0, 59)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 60 {
		t.Errorf("wildcard: expected 60 entries (0–59), got %d", len(got))
	}
	for i := 0; i <= 59; i++ {
		if !got[i] {
			t.Errorf("wildcard: missing value %d", i)
		}
	}
}

func TestParseCronField_Single(t *testing.T) {
	got, err := parseCronField("5", 0, 59)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || !got[5] {
		t.Errorf("single: expected {5}, got %v", got)
	}
}

func TestParseCronField_Range(t *testing.T) {
	got, err := parseCronField("2-4", 0, 59)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, v := range []int{2, 3, 4} {
		if !got[v] {
			t.Errorf("range: missing %d", v)
		}
	}
	if len(got) != 3 {
		t.Errorf("range: expected 3 entries, got %d", len(got))
	}
}

func TestParseCronField_CommaSeparated(t *testing.T) {
	got, err := parseCronField("1,3,5", 0, 59)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, v := range []int{1, 3, 5} {
		if !got[v] {
			t.Errorf("comma: missing %d", v)
		}
	}
	if len(got) != 3 {
		t.Errorf("comma: expected 3 entries, got %d", len(got))
	}
}

func TestParseCronField_StepOnWildcard(t *testing.T) {
	got, err := parseCronField("*/6", 0, 23)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{0, 6, 12, 18}
	for _, v := range want {
		if !got[v] {
			t.Errorf("*/6 [0-23]: missing %d", v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("*/6 [0-23]: expected %d entries, got %d", len(want), len(got))
	}
}

func TestParseCronField_StepOnRange(t *testing.T) {
	got, err := parseCronField("1-10/3", 0, 59)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{1, 4, 7, 10}
	for _, v := range want {
		if !got[v] {
			t.Errorf("1-10/3: missing %d", v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("1-10/3: expected %d entries, got %d", len(want), len(got))
	}
}

func TestParseCronField_StepInvalid(t *testing.T) {
	for _, bad := range []string{"*/0", "*/abc", "1-5/0"} {
		if _, err := parseCronField(bad, 0, 59); err == nil {
			t.Errorf("expected error for invalid step %q", bad)
		}
	}
}

func TestParseCronField_OutOfBounds(t *testing.T) {
	if _, err := parseCronField("60", 0, 59); err == nil {
		t.Error("expected error for out-of-bounds value 60")
	}
	if _, err := parseCronField("0-60", 0, 59); err == nil {
		t.Error("expected error for out-of-bounds range 0-60")
	}
}

func TestParseCronField_InvalidSyntax(t *testing.T) {
	for _, bad := range []string{"abc", "1-2-3", ""} {
		if _, err := parseCronField(bad, 0, 59); err == nil {
			t.Errorf("expected error for invalid field %q", bad)
		}
	}
}

// --- cronMatches ---

func TestCronMatches_EveryMinute(t *testing.T) {
	ts := time.Date(2026, 4, 4, 10, 30, 0, 0, time.UTC)
	ok, err := cronMatches("* * * * *", ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("* * * * * should match any time")
	}
}

func TestCronMatches_SpecificTime_Matches(t *testing.T) {
	// "30 10 4 4 6" = 10:30 on April 4th, Saturday (weekday 6)
	ts := time.Date(2026, 4, 4, 10, 30, 0, 0, time.UTC)
	ok, err := cronMatches("30 10 4 4 6", ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Errorf("expected match for %v with '30 10 4 4 6'", ts)
	}
}

func TestCronMatches_SpecificTime_NoMatch(t *testing.T) {
	// Wrong minute
	ts := time.Date(2026, 4, 4, 10, 31, 0, 0, time.UTC)
	ok, err := cronMatches("30 10 4 4 6", ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("should not match when minute differs")
	}
}

func TestCronMatches_Every6Hours(t *testing.T) {
	expr := "0 */6 * * *" // minute 0, every 6th hour: 00:00, 06:00, 12:00, 18:00
	matches := []time.Time{
		time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 4, 6, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 4, 18, 0, 0, 0, time.UTC),
	}
	nonMatches := []time.Time{
		time.Date(2026, 4, 4, 1, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 4, 6, 1, 0, 0, time.UTC),
	}
	for _, ts := range matches {
		ok, err := cronMatches(expr, ts)
		if err != nil {
			t.Fatalf("cronMatches(%q, %v): unexpected error: %v", expr, ts, err)
		}
		if !ok {
			t.Errorf("expected match for %v", ts)
		}
	}
	for _, ts := range nonMatches {
		ok, err := cronMatches(expr, ts)
		if err != nil {
			t.Fatalf("cronMatches(%q, %v): unexpected error: %v", expr, ts, err)
		}
		if ok {
			t.Errorf("expected no match for %v", ts)
		}
	}
}

func TestCronMatches_InvalidExpression(t *testing.T) {
	_, err := cronMatches("not a cron", time.Now())
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

func TestCronMatches_WrongFieldCount(t *testing.T) {
	_, err := cronMatches("* * * *", time.Now()) // only 4 fields
	if err == nil {
		t.Error("expected error for 4-field expression (need 5)")
	}
}
