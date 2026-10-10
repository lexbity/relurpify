// Package rewoo implements the ReWOO paradigm: plan, governed execution, synthesize.
//
// ReWOO runs in exactly three phases:
//
//  1. plan — when the task context carries no plan (rewoo.plan / plan keys),
//     the planner issues ONE LLM call with a strict-JSON plan schema; a
//     plan-parse failure fails the turn with ErrRewooPlanInvalid. A context
//     plan skips the LLM call entirely (the deterministic path), and an
//     authored plan (RewooOptions.AuthoredSteps from the recipe's plan/step
//     directives) is used verbatim with no planner call.
//  2. execute — tools run mechanically with no LLM involvement. Every step
//     passes the permission checker; a nil checker refuses execution with
//     ErrNoPermissionChecker, and a denied step follows the configured deny
//     policy (default: abort).
//  3. synthesize — one LLM call composes rewoo.final_output from the
//     aggregated step results. RewooOptions.Synthesize=false skips the call
//     and produces the deterministic mechanical summary instead (a documented
//     opt-out for mechanical pipelines, recorded as mode=mechanical).
//     RewooOptions.SynthesizeGuidance, when set, adds an authoritative
//     system instruction to the synthesizer prompt.
package rewoo
