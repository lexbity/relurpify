// Package identity provides network-facing identity resolution helpers for the
// middleware layer.
//
// This package is intentionally narrow. It owns bearer-token resolution,
// explicit tenant and subject lookup contracts, and typed resolution errors
// that network-facing servers can use to separate invalid credentials from
// backend lookup failures.
//
// Nexus should use this package as the identity boundary for gateway and other
// transport-facing authentication flows. Application-specific policy remains in
// the app layer or the relevant middleware consumer.
package identity
