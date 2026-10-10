package planner

import "errors"

// ErrInvalidModelOutput marks a planner model phase whose output could not be
// parsed into the required structure (a generated plan that is malformed or
// out of bounds, a verification verdict outside pass|fail, an empty summary).
// It is classified as model_invalid_output by the node boundary, whose
// protocol retries the phase once before applying the step's on_error policy.
// Quality outcomes (verification verdict "fail") are NOT errors.
var ErrInvalidModelOutput = errors.New("planner: invalid model output")
