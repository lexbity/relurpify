package cmd

import "testing"

func TestNewRootCmdUsesDevAgentName(t *testing.T) {
	root := NewRootCmd()
	if got := root.Use; got != "dev-agent" {
		t.Fatalf("root.Use = %q", got)
	}
	if got := root.Short; got != "Development and integration CLI for Relurpify" {
		t.Fatalf("root.Short = %q", got)
	}
}
