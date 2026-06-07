// Package search provides file glob and content search utilities for agents
// exploring a workspace.
//
// Glob pattern matching (glob.go) accepts standard doublestar patterns and
// returns matching file paths sorted by modification time. The unified search
// API (search.go) composes glob-based file discovery with content filtering
// to support the common agent workflow of locating relevant files before
// loading them into context.
package search
