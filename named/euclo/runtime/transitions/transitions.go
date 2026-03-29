package transitions

import (
	"time"

	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type UnitOfWorkTransitionState = runtimepkg.UnitOfWorkTransitionState
type UnitOfWorkHistoryEntry = runtimepkg.UnitOfWorkHistoryEntry
type UnitOfWork = runtimepkg.UnitOfWork

var ApplyUnitOfWorkTransition = runtimepkg.ApplyUnitOfWorkTransition

func Apply(existing UnitOfWork, next *UnitOfWork, now time.Time) UnitOfWorkTransitionState {
	return runtimepkg.ApplyUnitOfWorkTransition(existing, next, now)
}
