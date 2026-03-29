package local

import "github.com/lexcodex/relurpify/framework/core"

type migrationRollbackSnapshot struct {
	StepID      string
	Files       []map[string]any
	GitCommit   string
	Description string
}

func captureMigrationRollbackSnapshot(task *core.Task, step map[string]any) migrationRollbackSnapshot {
	snapshot := migrationRollbackSnapshot{
		StepID:      stringValue(step["id"]),
		GitCommit:   taskContextString(task, "git_commit"),
		Description: stringValue(step["rollback_path"]),
	}
	for _, file := range taskContextFiles(task) {
		snapshot.Files = append(snapshot.Files, map[string]any{
			"path":    file.Path,
			"content": file.Content,
		})
	}
	return snapshot
}

func restoreMigrationRollbackSnapshot(state *core.Context, snapshot migrationRollbackSnapshot) map[string]any {
	result := map[string]any{
		"step_id":         snapshot.StepID,
		"restored":        len(snapshot.Files) > 0,
		"restored_files":  len(snapshot.Files),
		"rollback_path":   snapshot.Description,
		"git_commit":      snapshot.GitCommit,
		"verification_ok": len(snapshot.Files) > 0,
	}
	if state == nil {
		result["restored"] = false
		result["verification_ok"] = false
		return result
	}
	state.Set("migration.rollback.snapshot", snapshot)
	if len(snapshot.Files) > 0 {
		state.Set("migration.rollback.restored_files", snapshot.Files)
	}
	return result
}
