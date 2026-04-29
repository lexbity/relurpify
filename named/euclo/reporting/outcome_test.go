package reporting

import (
	"testing"
)

func TestClassifyOutcomeSuccess(t *testing.T) {
	outcome := ClassifyOutcome(true, 0, false)

	if outcome.Category != OutcomeCategorySuccess {
		t.Errorf("Expected category success, got %s", outcome.Category)
	}

	if outcome.Reason != "execution completed successfully" {
		t.Errorf("Expected reason execution completed successfully, got %s", outcome.Reason)
	}

	if !outcome.Completed {
		t.Error("Expected completed to be true")
	}

	if outcome.ErrorCount != 0 {
		t.Errorf("Expected error count 0, got %d", outcome.ErrorCount)
	}
}

func TestClassifyOutcomeFailure(t *testing.T) {
	outcome := ClassifyOutcome(true, 2, false)

	if outcome.Category != OutcomeCategoryFailure {
		t.Errorf("Expected category failure, got %s", outcome.Category)
	}

	if outcome.Reason != "execution failed with errors" {
		t.Errorf("Expected reason execution failed with errors, got %s", outcome.Reason)
	}
}

func TestClassifyOutcomePartial(t *testing.T) {
	outcome := ClassifyOutcome(true, 1, false)

	if outcome.Category != OutcomeCategoryPartial {
		t.Errorf("Expected category partial, got %s", outcome.Category)
	}

	if outcome.Reason != "partial success with errors" {
		t.Errorf("Expected reason partial success with errors, got %s", outcome.Reason)
	}
}

func TestClassifyOutcomeBlocked(t *testing.T) {
	outcome := ClassifyOutcome(true, 0, true)

	if outcome.Category != OutcomeCategoryBlocked {
		t.Errorf("Expected category blocked, got %s", outcome.Category)
	}

	if outcome.Reason != "execution blocked by policy" {
		t.Errorf("Expected reason execution blocked by policy, got %s", outcome.Reason)
	}
}

func TestClassifyOutcomeCancelled(t *testing.T) {
	outcome := ClassifyOutcome(false, 0, false)

	if outcome.Category != OutcomeCategoryCancelled {
		t.Errorf("Expected category cancelled, got %s", outcome.Category)
	}

	if outcome.Reason != "execution cancelled" {
		t.Errorf("Expected reason execution cancelled, got %s", outcome.Reason)
	}
}

func TestClassifyOutcomeDetails(t *testing.T) {
	outcome := ClassifyOutcome(true, 0, false)

	if outcome.Details == nil {
		t.Error("Expected details to be initialized")
	}
}
