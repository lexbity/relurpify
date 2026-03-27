// Package context provides token budget tracking and adaptive compression for chainer execution.
//
// ChainerAgent can track LLM token usage and automatically compress context when
// the token budget is depleted, preventing runaway token consumption.
//
// # Budget Management
//
// BudgetManager wraps framework/contextmgr.ContextBudget and enforces limits:
//   - Warning threshold: trigger compression before hitting limit
//   - Exceeded threshold: apply aggressive compression or halt execution
//   - Per-category quotas: system, LLM, state, knowledge budgets
//
// # Compression Strategies
//
// CompressionListener reacts to budget events:
//   - OnBudgetWarning: apply adaptive compression (preserve first + recent)
//   - OnBudgetExceeded: apply aggressive compression or return error
//   - Automatic resumption: compressed state seamlessly resumes
//
// # Usage Pattern
//
// Enable budget tracking in ChainerAgent:
//
//	budget := context.NewBudgetManager(4096)  // 4K token limit
//	agent.BudgetManager = budget
//	agent.CompressionListener = context.NewCompressionListener()
//
// On execution, BudgetManager automatically:
//  1. Tracks token usage per stage
//  2. Emits warnings at threshold (e.g., 80%)
//  3. Compresses context on warning
//  4. Halts on critical overflow
package context
