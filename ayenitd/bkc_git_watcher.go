package ayenitd

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	archaeobkc "github.com/lexcodex/relurpify/archaeo/bkc"
	"github.com/lexcodex/relurpify/framework/sandbox"
)

// GitWatcherService polls git state and emits revision change events.
type GitWatcherService struct {
	WorkspaceRoot string
	EventBus      *archaeobkc.EventBus
	PollInterval  time.Duration
	LastRevision  string
	RunGit        func(context.Context, string, ...string) (string, error)
	Policy        sandbox.CommandPolicy

	mu     sync.Mutex
	cancel context.CancelFunc
}

func (s *GitWatcherService) Start(ctx context.Context) error {
	if s == nil || strings.TrimSpace(s.WorkspaceRoot) == "" {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	defer s.clearCancel()
	poll := s.PollInterval
	if poll <= 0 {
		poll = 30 * time.Second
	}
	current, err := s.currentRevision(runCtx)
	if err != nil {
		return nil
	}
	if strings.TrimSpace(s.LastRevision) == "" {
		s.LastRevision = current
	}
	if current != s.LastRevision {
		paths, pathErr := s.affectedPaths(runCtx, current)
		if pathErr == nil && len(paths) > 0 {
			s.emitRevisionChanged(current, paths)
		}
		s.LastRevision = current
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-runCtx.Done():
			return nil
		case <-ticker.C:
			next, err := s.currentRevision(runCtx)
			if err != nil || next == "" || next == s.LastRevision {
				continue
			}
			paths, err := s.affectedPaths(runCtx, next)
			if err != nil {
				continue
			}
			s.emitRevisionChanged(next, paths)
			s.LastRevision = next
		}
	}
}

func (s *GitWatcherService) Stop() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *GitWatcherService) currentRevision(ctx context.Context) (string, error) {
	out, err := s.runGit(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (s *GitWatcherService) affectedPaths(ctx context.Context, revision string) ([]string, error) {
	out, err := s.runGit(ctx, "diff-tree", "--no-commit-id", "--name-only", "-r", revision)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	outPaths := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			outPaths = append(outPaths, line)
		}
	}
	return outPaths, nil
}

func (s *GitWatcherService) runGit(ctx context.Context, args ...string) (string, error) {
	if s.Policy != nil {
		if err := s.Policy.AllowCommand(ctx, sandbox.CommandRequest{
			Workdir: s.WorkspaceRoot,
			Args:    append([]string{"git"}, args...),
		}); err != nil {
			return "", err
		}
	}
	if s.RunGit != nil {
		return s.RunGit(ctx, s.WorkspaceRoot, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.WorkspaceRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (s *GitWatcherService) emitRevisionChanged(revision string, paths []string) {
	if s.EventBus == nil {
		return
	}
	s.EventBus.EmitCodeRevisionChanged(archaeobkc.CodeRevisionChangedPayload{
		WorkspaceRoot: s.WorkspaceRoot,
		NewRevision:   revision,
		AffectedPaths: append([]string(nil), paths...),
	})
}

func (s *GitWatcherService) clearCancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel = nil
}
