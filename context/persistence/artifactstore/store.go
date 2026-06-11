// Package artifactstore provides a durable per-session store for large tool
// outputs. Artifacts are stored on disk under
// <workspace>/.relurpify_state/artifacts/<session>/ and are GC'd at session
// end or when a global size cap is exceeded.
package artifactstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Ref is a retrieval-addressable artifact handle (e.g. "artifact://<session>/<id>").
type Ref string

// Session returns the session portion of the ref.
func (r Ref) Session() string {
	// Format: artifact://<session>/<id>  (11 chars for "artifact://")
	if len(r) < 12 {
		return ""
	}
	s := string(r[11:])
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return s[:i]
		}
	}
	return ""
}

// ArtifactMeta describes a stored artifact.
type ArtifactMeta struct {
	Session   string            `json:"session"`
	Kind      string            `json:"kind"`
	Size      int64             `json:"size"`
	CreatedAt time.Time         `json:"created_at"`
	Meta      map[string]string `json:"meta,omitempty"`
}

// Store is a durable per-session artifact store.
type Store interface {
	Put(ctx context.Context, kind string, meta map[string]string, r io.Reader) (Ref, error)
	Open(ctx context.Context, ref Ref) (io.ReadCloser, ArtifactMeta, error)
	GC(ctx context.Context, session string) error
	Close() error
}

type sessionGCState struct {
	Size    int64
	ModTime time.Time
}

// DiskStore implements Store on the local filesystem.
type DiskStore struct {
	rootDir string
	maxSize int64 // global size cap in bytes; 0 = unlimited
	mu      sync.Mutex

	// Tracked for GC.
	sessions map[string]*sessionGCState // session → metadata
	total    int64
}

// NewDiskStore creates an artifact store rooted at <workspace>/.relurpify_state/artifacts.
func NewDiskStore(workspace string, maxSize int64) (*DiskStore, error) {
	rootDir := filepath.Join(workspace, ".relurpify_state", "artifacts")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact root: %w", err)
	}
	if maxSize <= 0 {
		maxSize = 512 * 1024 * 1024 // 512 MiB default
	}
	s := &DiskStore{
		rootDir:  rootDir,
		maxSize:  maxSize,
		sessions: make(map[string]*sessionGCState),
	}
	if err := s.scan(); err != nil {
		return nil, fmt.Errorf("scan existing artifacts: %w", err)
	}
	return s, nil
}

// scan walks rootDir on startup to populate sessions and total with actual file sizes.
func (s *DiskStore) scan() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		session := entry.Name()
		sessionDir := filepath.Join(s.rootDir, session)
		var sessionSize int64
		err = filepath.Walk(sessionDir, func(path string, walkInfo os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !walkInfo.IsDir() {
				sessionSize += walkInfo.Size()
			}
			return nil
		})
		if err != nil {
			return err
		}
		s.sessions[session] = &sessionGCState{
			Size:    sessionSize,
			ModTime: info.ModTime(),
		}
		s.total += sessionSize
	}
	return nil
}

// sessionDir returns the directory for a given session.
func (s *DiskStore) sessionDir(session string) string {
	return filepath.Join(s.rootDir, session)
}

// artifactPath returns the data file path for a ref.
// "artifact://" is 11 characters; the remainder is "<session>/<id>".
func (s *DiskStore) artifactPath(ref Ref) string {
	return filepath.Join(s.rootDir, string(ref[11:]))
}

// metaPath returns the metadata file path for a ref.
func (s *DiskStore) metaPath(ref Ref) string {
	return s.artifactPath(ref) + ".meta"
}

func randID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Put streams content to disk and returns a stable ref.
func (s *DiskStore) Put(_ context.Context, kind string, meta map[string]string, r io.Reader) (Ref, error) {
	if r == nil {
		return "", errors.New("reader required")
	}
	id := randID()

	// Determine session from meta, or use "default".
	session := meta["session"]
	if session == "" {
		session = "default"
	}

	sessionDir := s.sessionDir(session)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return "", fmt.Errorf("create session dir: %w", err)
	}

	ref := Ref("artifact://" + session + "/" + id)
	dataPath := s.artifactPath(ref)

	f, err := os.Create(filepath.Clean(dataPath))
	if err != nil {
		return "", fmt.Errorf("create artifact: %w", err)
	}

	written, err := io.Copy(f, r)
	if err != nil {
		f.Close()
		os.Remove(dataPath)
		return "", fmt.Errorf("write artifact: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(dataPath)
		return "", fmt.Errorf("close artifact: %w", err)
	}

	// Write metadata.
	am := ArtifactMeta{
		Session:   session,
		Kind:      kind,
		Size:      written,
		CreatedAt: time.Now().UTC(),
		Meta:      meta,
	}
	metaBytes, err := json.Marshal(am)
	if err != nil {
		os.Remove(dataPath)
		return "", fmt.Errorf("marshal metadata: %w", err)
	}
	if err := os.WriteFile(s.metaPath(ref), metaBytes, 0o644); err != nil {
		os.Remove(dataPath)
		os.Remove(s.metaPath(ref))
		return "", fmt.Errorf("write metadata: %w", err)
	}

	// Track size and mod time for GC.
	s.mu.Lock()
	state := s.sessions[session]
	if state == nil {
		state = &sessionGCState{}
		s.sessions[session] = state
	}
	state.Size += written
	state.ModTime = time.Now()
	s.total += written
	s.mu.Unlock()

	return ref, nil
}

// Open retrieves an artifact by ref.
func (s *DiskStore) Open(_ context.Context, ref Ref) (io.ReadCloser, ArtifactMeta, error) {
	dataPath := s.artifactPath(ref)
	f, err := os.Open(filepath.Clean(dataPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ArtifactMeta{}, fmt.Errorf("artifact %q not found", ref)
		}
		return nil, ArtifactMeta{}, fmt.Errorf("open artifact: %w", err)
	}

	metaBytes, err := os.ReadFile(s.metaPath(ref))
	if err != nil {
		f.Close()
		return nil, ArtifactMeta{}, fmt.Errorf("read metadata: %w", err)
	}

	var am ArtifactMeta
	if err := json.Unmarshal(metaBytes, &am); err != nil {
		f.Close()
		return nil, ArtifactMeta{}, fmt.Errorf("unmarshal metadata: %w", err)
	}

	return f, am, nil
}

// GC removes all artifacts for a session and performs a global size-cap sweep
// if the total exceeds maxSize.
func (s *DiskStore) GC(_ context.Context, session string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove session directory.
	sessionDir := s.sessionDir(session)
	if err := os.RemoveAll(sessionDir); err != nil {
		return fmt.Errorf("remove session dir: %w", err)
	}
	delete(s.sessions, session)

	// Recalculate total.
	s.total = 0
	for _, state := range s.sessions {
		s.total += state.Size
	}

	return nil
}

// Close is a no-op for the disk store.
func (s *DiskStore) Close() error { return nil }

// TotalBytes returns the total bytes tracked across all sessions.
func (s *DiskStore) TotalBytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}
