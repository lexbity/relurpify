// Package anitd provides the composition root and service lifecycle manager for Relurpify.
// It is analogous to systemd/init: it starts services in dependency order, holds them alive,
// and shuts them down cleanly on exit.
//
// The main entry point is Open(), which initializes a complete workspace session.
// Nothing in agents/, named/, or app/ should be responsible for constructing or wiring
// platform services — that is anitd's job.
package anitd
