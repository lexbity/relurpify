package session

import (
	"context"
	"errors"
)

// CloseStack manages ordered resource cleanup in reverse registration order.
// Use during multi-stage construction to ensure partial failures close all
// previously-opened resources. Zero value is ready to use.
type CloseStack struct {
	items []func(context.Context) error
}

// Add registers a close function. Nil functions are silently skipped.
// Close functions are called in reverse order of registration.
func (s *CloseStack) Add(fn func(context.Context) error) {
	if fn != nil {
		s.items = append(s.items, fn)
	}
}

// Close invokes all registered close functions in reverse registration order.
// Errors are aggregated via errors.Join. The stack is cleared after Close
// returns, making subsequent Close calls no-ops. Nil-safe.
func (s *CloseStack) Close(ctx context.Context) error {
	if s == nil || len(s.items) == 0 {
		return nil
	}
	var err error
	for i := len(s.items) - 1; i >= 0; i-- {
		if fn := s.items[i]; fn != nil {
			err = errors.Join(err, fn(ctx))
		}
	}
	s.items = nil
	return err
}

// Len returns the number of items currently registered in the stack.
func (s *CloseStack) Len() int {
	if s == nil {
		return 0
	}
	return len(s.items)
}
