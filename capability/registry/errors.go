package registry

import (
	"errors"
	"time"
)

// ErrRollbackExpired is returned when a rollback token is no longer
// present — evicted at the retention cap or past its TTL. The rollback
// window is deliberately finite; callers surface this as documented
// behavior, not an internal fault.
var ErrRollbackExpired = errors.New("rollback window expired")

// Rollback retention bounds: tokens beyond the cap or older than the TTL are
// evicted (oldest first) and later rollbacks fail with ErrRollbackExpired.
const (
	rollbackTokenCap = 256
	rollbackTokenTTL = 30 * time.Minute
)
