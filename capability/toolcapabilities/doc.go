// Package toolcapabilities governs local tool admission, builds tool
// implementations from manifests, and enforces manifest/implementation
// consistency checks such as parameter-key drift detection.
//
// This package handles only subprocess, go_native, and composite backends.
// Tools with other backends are handled by their respective subsystems.
//
// Layer constraint: capability/toolcapabilities may import platform tool
// implementations through approved registration points, but it must not route
// through the old platform/shell CLI wrappers.
package toolcapabilities
