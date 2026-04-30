// dev-agent is the development and scripted CLI for driving Relurpify agents
// directly from the command line.
//
// This binary is intended for development, testing, and scripted automation.
// End-user coding is done via the relurpish TUI.
//
// # Commands
//
//   - start: begin an agent session with a given instruction and agent type.
//   - workspace: inspect workspace config, probes, and services.
//   - service: manage workspace services.
//   - agenttest: run integration test suites with optional tape recording and
//     replay for deterministic CI.
//   - agents: list registered agent types.
//   - skill: inspect and manage skill packages.
//   - session: list and inspect past sessions.
//   - config: display resolved workspace configuration.
//
// # Flags
//
//   - --agent: agent type to use (default: coding).
//   - --mode: execution mode for euclo and other agents.
//   - --instruction: task instruction (required for start).
//   - --yes: approve all HITL prompts automatically (Allow policy).
//   - --json: emit a machine-readable completion summary.
package main
