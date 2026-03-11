// Package search provides code search and content indexing tools that agents
// use to locate relevant files and symbols within a workspace.
//
// The search tool supports pattern-based file search (glob), content search
// (ripgrep-style), and symbol lookup via the AST index. Results are ranked
// by relevance and returned as structured observations the agent can act on
// without loading entire files into context.
package search
