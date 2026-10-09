package rewoo

import "errors"

var (
	// ErrNoPermissionChecker is returned when a ReWOO execution is attempted
	// without a permission checker. Governance is unconditional: a ReWOO turn
	// without governance fails loudly instead of running ungoverned.
	ErrNoPermissionChecker = errors.New("rewoo: permission checker required")

	// ErrRewooPlanInvalid is returned when the planner's LLM output cannot be
	// decoded into a usable plan. The turn fails loudly — there is no silent
	// fallback to single-step execution.
	ErrRewooPlanInvalid = errors.New("rewoo: planner produced an invalid plan")
)
