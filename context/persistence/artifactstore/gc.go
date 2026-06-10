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

	s.mu.Lock()
	now := time.Now()
	result := &GCResult{}

	type victimInfo struct {
		name string
		size int64
	}
	var victims []victimInfo

	for name, state := range s.sessions {
		if now.Sub(state.ModTime) > maxAge {
			victims = append(victims, victimInfo{
				name: name,
				size: state.Size,
			})
		}
	}
	s.mu.Unlock()

	for _, victim := range victims {
		s.mu.Lock()
		state, exists := s.sessions[victim.name]
		if !exists {
			s.mu.Unlock()
			continue
		}
		delete(s.sessions, victim.name)
		s.total -= state.Size
		s.mu.Unlock()

		sessionDir := filepath.Join(s.rootDir, victim.name)
		if err := os.RemoveAll(sessionDir); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("remove %s: %v", victim.name, err))
			continue
		}
		result.SessionsRemoved++
		result.BytesFreed += state.Size
	}

	return result, nil
}

// EvictOldest removes the oldest artifact sessions until total size is below
// the target. Sessions are ordered by modification time (oldest first).
func (s *DiskStore) EvictOldest(ctx context.Context, targetBytes int64) (*GCResult, error) {
	s.mu.Lock()
	total := s.total
	if total <= targetBytes {
		s.mu.Unlock()
		return &GCResult{}, nil
	}

	type sessionInfo struct {
		name string
		size int64
		mod  time.Time
	}

	var sessions []sessionInfo
	for name, state := range s.sessions {
		sessions = append(sessions, sessionInfo{
			name: name,
			size: state.Size,
			mod:  state.ModTime,
		})
	}
	s.mu.Unlock()

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
		state, exists := s.sessions[sess.name]
		if !exists {
			s.mu.Unlock()
			continue
		}
		delete(s.sessions, sess.name)
		s.total -= state.Size
		s.mu.Unlock()

		sessionDir := filepath.Join(s.rootDir, sess.name)
		if err := os.RemoveAll(sessionDir); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("remove %s: %v", sess.name, err))
			continue
		}
		result.SessionsRemoved++
		result.BytesFreed += state.Size
		needToFree -= state.Size
	}

	return result, nil
}
