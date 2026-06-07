package artifactstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// GCResult summarizes a GC run.
type GCResult struct {
	SessionsRemoved int
	BytesFreed      int64
	Errors          []string
}

// GCAge removes artifact sessions older than maxAge. This is a global sweep
// across all sessions in the store.
func (s *DiskStore) GCAge(ctx context.Context, maxAge time.Duration) (*GCResult, error) {
	if maxAge <= 0 {
		return &GCResult{}, nil
	}

	entries, err := os.ReadDir(s.rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &GCResult{}, nil
		}
		return nil, fmt.Errorf("read artifact root: %w", err)
	}

	now := time.Now()
	result := &GCResult{}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("stat %s: %v", entry.Name(), err))
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			sessionDir := filepath.Join(s.rootDir, entry.Name())
			var size int64
			filepath.Walk(sessionDir, func(path string, fi os.FileInfo, err error) error {
				if err == nil && !fi.IsDir() {
					size += fi.Size()
				}
				return nil
			})
			if err := os.RemoveAll(sessionDir); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("remove %s: %v", entry.Name(), err))
				continue
			}
			result.SessionsRemoved++
			result.BytesFreed += size
		}
	}

	return result, nil
}

// EvictOldest removes the oldest artifact sessions until total size is below
// the target. Sessions are ordered by modification time (oldest first).
func (s *DiskStore) EvictOldest(ctx context.Context, targetBytes int64) (*GCResult, error) {
	s.mu.Lock()
	total := s.total
	s.mu.Unlock()

	if total <= targetBytes {
		return &GCResult{}, nil
	}

	entries, err := os.ReadDir(s.rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &GCResult{}, nil
		}
		return nil, fmt.Errorf("read artifact root: %w", err)
	}

	type sessionInfo struct {
		name string
		size int64
		mod  time.Time
	}

	var sessions []sessionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		sessionDir := filepath.Join(s.rootDir, entry.Name())
		var size int64
		filepath.Walk(sessionDir, func(path string, fi os.FileInfo, err error) error {
			if err == nil && !fi.IsDir() {
				size += fi.Size()
			}
			return nil
		})
		sessions = append(sessions, sessionInfo{
			name: entry.Name(),
			size: size,
			mod:  info.ModTime(),
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].mod.Before(sessions[j].mod)
	})

	result := &GCResult{}
	needToFree := total - targetBytes

	for _, sess := range sessions {
		if needToFree <= 0 {
			break
		}
		s.mu.Lock()
		delete(s.sessions, sess.name)
		s.total -= sess.size
		s.mu.Unlock()

		sessionDir := filepath.Join(s.rootDir, sess.name)
		if err := os.RemoveAll(sessionDir); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("remove %s: %v", sess.name, err))
			continue
		}
		result.SessionsRemoved++
		result.BytesFreed += sess.size
		needToFree -= sess.size
	}

	return result, nil
}
