package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/config"
)

// SessionMeta holds lightweight session metadata for listing.
type SessionMeta struct {
	ID        string    `json:"id"`
	StartTime time.Time `json:"start_time"`
	UpdatedAt time.Time `json:"updated_at"`
	Workspace string    `json:"workspace"`
	Agent     string    `json:"agent"`
	Model     string    `json:"model"`
	Label     string    `json:"label,omitempty"` // set for named checkpoints
}

// SessionRecord is the full persisted session.
type SessionRecord struct {
	SessionMeta
	Context  *AgentContext `json:"context,omitempty"`
	Messages []Message     `json:"messages,omitempty"`
}

// SessionStore persists TUI sessions to disk.
type SessionStore struct {
	root string
}

// NewSessionStore creates a store rooted at dir.
func NewSessionStore(workspace string) *SessionStore {
	root := config.New(workspace).SessionsDir()
	return &SessionStore{root: root}
}

func (s *SessionStore) sessionDir(id string) string {
	return filepath.Join(s.root, id)
}

func (s *SessionStore) sessionFile(id string) string {
	return filepath.Join(s.sessionDir(id), "session.json")
}

// Save writes a session record to disk.
func (s *SessionStore) Save(rec SessionRecord) error {
	if rec.ID == "" {
		return fmt.Errorf("session id required")
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = time.Now()
	}
	dir := s.sessionDir(rec.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.sessionFile(rec.ID), data, 0o644)
}

// Load reads a session record by ID.
func (s *SessionStore) Load(id string) (SessionRecord, error) {
	data, err := os.ReadFile(s.sessionFile(id))
	if err != nil {
		return SessionRecord{}, err
	}
	var rec SessionRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return SessionRecord{}, err
	}
	return rec, nil
}

// List returns all session metas (excluding checkpoints) sorted newest first.
func (s *SessionStore) List() ([]SessionMeta, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var metas []SessionMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Skip checkpoints; they are listed separately via ListCheckpoints()
		if strings.HasPrefix(e.Name(), checkpointPrefix) {
			continue
		}
		f := filepath.Join(s.root, e.Name(), "session.json")
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var rec SessionRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		metas = append(metas, rec.SessionMeta)
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].UpdatedAt.After(metas[j].UpdatedAt)
	})
	return metas, nil
}

// Latest returns the most recently updated session, if any.
func (s *SessionStore) Latest() (SessionRecord, bool, error) {
	metas, err := s.List()
	if err != nil {
		return SessionRecord{}, false, err
	}
	if len(metas) == 0 {
		return SessionRecord{}, false, nil
	}
	rec, err := s.Load(metas[0].ID)
	if err != nil {
		return SessionRecord{}, false, err
	}
	return rec, true, nil
}

// Delete removes a session directory.
func (s *SessionStore) Delete(id string) error {
	return os.RemoveAll(s.sessionDir(id))
}

const checkpointPrefix = "ckpt-"

// SaveCheckpoint persists a named checkpoint with a unique ID based on label and timestamp.
func (s *SessionStore) SaveCheckpoint(rec SessionRecord) error {
	ts := time.Now().Format("20060102-150405")
	label := rec.Label
	if label == "" {
		label = ts
	}
	rec.ID = fmt.Sprintf("%s%s-%s", checkpointPrefix, label, ts)
	rec.UpdatedAt = time.Now()
	return s.Save(rec)
}

// ListCheckpoints returns metas for checkpoint records only, sorted newest first.
func (s *SessionStore) ListCheckpoints() ([]SessionMeta, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var metas []SessionMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only include checkpoints
		if !strings.HasPrefix(e.Name(), checkpointPrefix) {
			continue
		}
		f := filepath.Join(s.root, e.Name(), "session.json")
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var rec SessionRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		metas = append(metas, rec.SessionMeta)
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].UpdatedAt.After(metas[j].UpdatedAt)
	})
	return metas, nil
}
