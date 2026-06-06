// Package context implements context-shaping policy evaluation for recipe
// steps — replacing the retired framework/contextpolicy package.
//
// ContextPolicyBundle carries the compiled policy configuration (rankers,
// scanners, summarizers, quotas, rate limits) that ingestion, persistence,
// retrieval, and the compiler consult at runtime.
package context
