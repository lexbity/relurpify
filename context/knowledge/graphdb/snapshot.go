package graphdb

import (
	"encoding/json"
	"errors"
	"os"
)

type snapshotState struct {
	Nodes           []NodeRecord              `json:"nodes"`
	Forward         []EdgeRecord              `json:"forward"`
	NodeHistory     map[string][]NodeRecord   `json:"node_history,omitempty"`
	EdgeHistory     map[string][]EdgeRecord   `json:"edge_history,omitempty"`
	MutationResults map[string]MutationResult `json:"mutation_results,omitempty"`
}

func writeSnapshot(path string, state snapshotState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readSnapshot(path string) (snapshotState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return snapshotState{}, nil
	}
	if err != nil {
		return snapshotState{}, err
	}
	if len(data) == 0 {
		return snapshotState{}, nil
	}
	var state snapshotState
	if err := json.Unmarshal(data, &state); err != nil {
		return snapshotState{}, err
	}
	return state, nil
}
