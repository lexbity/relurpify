package runtime

import (
	"os"
	"path/filepath"
	"time"

	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/securefile"
)

// UIRecoveryStore checkpoints unaccepted composer drafts locally with
// owner-only permissions (spec D-18). It never holds permission grants, run
// state, or anything authoritative: losing it loses nothing but typing.
type UIRecoveryStore struct {
	root string // <workspace>/.relurpify_state/ui
	now  func() time.Time
}

// NewUIRecoveryStore binds draft recovery to one workspace state directory.
func NewUIRecoveryStore(workspaceRoot string) *UIRecoveryStore {
	return &UIRecoveryStore{
		root: filepath.Join(config.DefaultWorkspaceStateDir(workspaceRoot), "ui"),
		now:  time.Now,
	}
}

const maxRecoveryDraftBytes = 64 << 10 // spec §8.5 bar input bound

// SaveDraft persists the unsent composer text. Failures are reported, never
// fatal: the in-memory draft always remains the truth.
func (s *UIRecoveryStore) SaveDraft(text string) error {
	if len(text) > maxRecoveryDraftBytes {
		text = text[:maxRecoveryDraftBytes]
	}
	if err := securefile.MkdirAllSecure(s.root); err != nil {
		return err
	}
	destination := filepath.Join(s.root, "composer-draft")
	if text == "" {
		_ = os.Remove(destination)
		return nil
	}
	wrapped := "relurpify ui draft/1\nsaved_at: " + s.now().UTC().Format(time.RFC3339) + "\n---\n" + text
	return securefile.WriteFileSecure(destination, []byte(wrapped))
}

// LoadDraft restores the last checkpointed draft, or empty when none exists.
func (s *UIRecoveryStore) LoadDraft() (string, error) {
	data, err := os.ReadFile(filepath.Join(s.root, "composer-draft")) //nolint:gosec // fixed confined path below the workspace state root
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	const marker = "\n---\n"
	idx := indexMarker(string(data), marker)
	if idx < 0 {
		return "", nil
	}
	draft := string(data[idx+len(marker):])
	if len(draft) > maxRecoveryDraftBytes {
		draft = draft[:maxRecoveryDraftBytes]
	}
	return draft, nil
}

func indexMarker(s, marker string) int {
	for i := len(s) - len(marker); i >= 0; i-- {
		if s[i:i+len(marker)] == marker {
			return i
		}
	}
	return -1
}

// ClearDraft removes any persisted draft (used after submission or when the
// user disables draft persistence).
func (s *UIRecoveryStore) ClearDraft() error {
	err := os.Remove(filepath.Join(s.root, "composer-draft"))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}
