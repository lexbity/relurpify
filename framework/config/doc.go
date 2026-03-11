// Package config resolves workspace configuration paths for the Relurpify
// runtime.
//
// paths.go provides helpers that locate the canonical relurpify_cfg/ directory
// relative to the project root, derive paths for agent manifests, skill
// packages, session storage, telemetry output, and log files, and ensure
// required directories exist before the runtime starts.
package config
