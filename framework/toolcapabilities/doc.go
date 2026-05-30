// Package toolcapabilities governs local tool admission, builds tool
// implementations from manifests, and enforces manifest/implementation
// consistency checks such as parameter-key drift detection.
//
// This package handles only subprocess, go_native, and composite backends.
// MCP tools are routed to a different subsystem and must NOT be processed here.
//
// Layer constraint: framework/toolcapabilities imports from platform/tools/*
// but NOT from platform/shell/cli_* (which is the legacy path being replaced).
package toolcapabilities
