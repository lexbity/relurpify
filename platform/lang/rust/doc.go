// Package rust provides Rust language tools for agents working on Rust
// codebases.
//
// Registered capability tools include: cargo build, cargo test (with filter
// support), cargo clippy, cargo fmt, and cargo check. Each tool runs via the
// sandbox runner and returns structured compiler and test output as agent
// observations.
package rust
