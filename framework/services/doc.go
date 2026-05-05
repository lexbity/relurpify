// Package services provides framework-level service construction functions.
//
// This package builds core framework services (capability bundles, prompt registries)
// without any knowledge of specific named agents or the ayenitd service runner.
//
// Ownership:
//   - framework/services owns framework service construction
//   - framework/services must not import ayenitd
//   - framework/services must not import named
//   - framework/services may import framework/* and platform/*
package services
