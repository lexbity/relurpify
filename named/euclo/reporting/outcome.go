package reporting

// ClassifyOutcome determines the outcome category based on execution results.
func ClassifyOutcome(completed bool, errorCount int, blocked bool) *Outcome {
	outcome := &Outcome{
		Details:    map[string]string{},
		Completed: completed,
		ErrorCount: errorCount,
	}

	if blocked {
		outcome.Category = OutcomeCategoryBlocked
		outcome.Reason = "execution blocked by policy"
		return outcome
	}

	if !completed {
		outcome.Category = OutcomeCategoryCancelled
		outcome.Reason = "execution cancelled"
		return outcome
	}

	if errorCount > 0 {
		if errorCount == 1 {
			outcome.Category = OutcomeCategoryPartial
			outcome.Reason = "partial success with errors"
		} else {
			outcome.Category = OutcomeCategoryFailure
			outcome.Reason = "execution failed with errors"
		}
		return outcome
	}

	outcome.Category = OutcomeCategorySuccess
	outcome.Reason = "execution completed successfully"
	return outcome
}
