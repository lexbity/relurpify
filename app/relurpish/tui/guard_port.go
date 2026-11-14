package tui

import (
	"context"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// GuardPolicySnapshot contains inspected source. It establishes no live enforcement.
type GuardPolicySnapshot struct {
	Workspace, Notice string
	Document          config.GuardPolicyDocument
}

// GuardPolicyPort is deliberately read-only until shell grammar and ingestion
// enforcement are implemented. Neither inspection nor pattern tests grant rights.
type GuardPolicyPort interface {
	GuardPolicy(context.Context, config.GuardSection) (GuardPolicySnapshot, error)
}
