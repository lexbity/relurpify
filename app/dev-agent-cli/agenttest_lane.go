package main

import "fmt"

type agentTestLanePreset struct {
	Tier               string
	Profile            string
	Strict             bool
	IncludeQuarantined bool
}

func resolveAgentTestLane(name string) (agentTestLanePreset, error) {
	switch name {
	case "":
		return agentTestLanePreset{}, nil
	case "pr-smoke":
		return agentTestLanePreset{
			Tier:    "smoke",
			Profile: "ci-live",
			Strict:  true,
		}, nil
	case "merge-stable":
		return agentTestLanePreset{
			Tier:    "stable",
			Profile: "ci-live",
			Strict:  true,
		}, nil
	case "quarantined-live":
		return agentTestLanePreset{
			Profile:            "ci-live",
			IncludeQuarantined: true,
			Strict:             true,
		}, nil
	default:
		return agentTestLanePreset{}, fmt.Errorf("unknown agenttest lane %q", name)
	}
}
