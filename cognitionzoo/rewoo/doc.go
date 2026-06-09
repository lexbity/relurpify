// Package rewoo implements a small-model-friendly ReWOO execution agent.
//
// ReWOO runs in three phases:
//  1. plan with a single LLM call
//  2. execute tools mechanically with no LLM involvement
//  3. synthesize a final answer with a second LLM call
package rewoo
