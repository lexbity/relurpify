package runtime

import (
	"context"
	"crypto/sha256"
	"sync/atomic"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
)

// RunEventKind classifies one streamed run event.
type RunEventKind string

const (
	// RunEventStarted announces that the executor took the run.
	RunEventStarted RunEventKind = "started"
	// RunEventToken carries one coalesced stream preview token.
	RunEventToken RunEventKind = "token"
	// RunEventAwaitingInteraction pauses the ticket at a decision boundary.
	RunEventAwaitingInteraction RunEventKind = "awaiting_interaction"
	// RunEventFinished is the exactly-once terminal event; the channel closes after it.
	RunEventFinished RunEventKind = "finished"
)

// RunOutcome is the terminal classification delivered with RunEventFinished.
type RunOutcome string

const (
	RunOutcomeCompleted  RunOutcome = "completed"
	RunOutcomeFailed     RunOutcome = "failed"
	RunOutcomeCanceled   RunOutcome = "canceled"
	RunOutcomeInterrupted RunOutcome = "interrupted"
)

// InstructionCommand is one admitted instruction. The journal digest covers
// the canonical payload so an exact UI retry resolves to the same run.
type InstructionCommand struct {
	Instruction string
	TaskType    execution.TaskType
	Metadata    map[string]any
	// RequestID is the client-generated idempotency identity; empty generates one.
	RequestID RequestID
}

// RunEvent is one observation delivered to a ticket subscriber. Token events
// are bounded previews; the committed journal, not this stream, is authority.
type RunEvent struct {
	RunID    string
	Kind     RunEventKind
	Token    string
	Outcome  RunOutcome
	Awaiting bool
	Err      error
	Result   *execution.Result
	Duration time.Duration
	// Dropped counts token previews this dispatcher dropped because the
	// subscriber could not keep up. Committed facts are never dropped.
	Dropped int64
}

// RunTicket is one subscriber handle to a runtime-owned run. The runtime owns
// the executor goroutine and its context for the run's whole life; the ticket
// only observes and may request a protective stop. Navigation away never
// tears down the run: closing or abandoning the ticket changes nothing.
type RunTicket struct {
	// RunID is the durable journal identity of the run.
	RunID string
	// Events delivers observations until the exactly-once finished event,
	// then closes.
	Events <-chan RunEvent
	// Cancel requests a protective stop for the run. It reports whether the
	// stop was delivered to a live executor; a durable terminal outcome is
	// committed by the executor, never by Cancel.
	Cancel func() (stopDelivered bool)
}

// runSubmitOutcome reports the durable admission result.
type runSubmitOutcome struct {
	RunID  string
	Status ReceiptStatus
}

// RunService is the workspace owner's execution dispatcher: it admits runs
// durably, owns their executor goroutines and contexts, journals every
// transition, and reconciles abandoned runs at boot. Views never own a run.
type RunService struct {
	rt      *Runtime
	coord   *RunCoordinator
	journal *RunJournal
	timeout time.Duration
	// exec is the turn executor. It defaults to the runtime's submitTurn;
	// the field is set once at construction and never mutated afterwards.
	exec func(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error)

	mu   sync.Mutex
	live map[string]*serviceRun
}

type serviceRun struct {
	runID   string
	cancel  context.CancelFunc
	ch      chan RunEvent
	closed  sync.Once
	dropped atomic.Int64
}

// NewRunService wires the dispatcher to its runtime, coordinator and journal.
func NewRunService(rt *Runtime, coord *RunCoordinator, journal *RunJournal) (*RunService, error) {
	if rt == nil || coord == nil || journal == nil {
		return nil, errors.New("run service: runtime, coordinator and journal required")
	}
	svc := &RunService{rt: rt, coord: coord, journal: journal, timeout: 15 * time.Minute, live: make(map[string]*serviceRun)}
	svc.exec = rt.submitTurn
	return svc, nil
}

// SetTimeout bounds one run's wall clock. Zero restores the default.
func (s *RunService) SetTimeout(d time.Duration) {
	if d > 0 {
		s.timeout = d
	}
}

// instructionDigest is the canonical payload digest for admission retries.
func instructionDigest(cmd InstructionCommand) string {
	keys := make([]string, 0, len(cmd.Metadata))
	for k := range cmd.Metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	h.Write([]byte(cmd.Instruction))
	h.Write([]byte{0})
	h.Write([]byte(cmd.TaskType))
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0x1f})
		enc, _ := json.Marshal(cmd.Metadata[k])
		h.Write(enc)
		h.Write([]byte{0x1e})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Submit admits the instruction durably and starts the runtime-owned executor.
// An exact retry of the same RequestID resolves to the original run: for a run
// still queued it starts the executor exactly once; a run past queued replays
// no events and reports its committed state through a finished event.
func (s *RunService) Submit(ctx context.Context, cmd InstructionCommand) (runSubmitOutcome, *RunTicket, error) {
	if s == nil || s.rt == nil {
		return runSubmitOutcome{}, nil, errors.New("run service unavailable")
	}
	if strings.TrimSpace(cmd.Instruction) == "" {
		return runSubmitOutcome{}, nil, errors.New("instruction required")
	}
	digest := instructionDigest(cmd)
	snap, err := s.journal.AdmitRun(ctx, cmd.RequestID, digest, cmd.Instruction)
	if err != nil {
		return runSubmitOutcome{}, nil, err
	}
	outcome := runSubmitOutcome{RunID: snap.RunID, Status: ReceiptStatusAccepted}

	s.mu.Lock()
	_, live := s.live[snap.RunID]
	s.mu.Unlock()
	if live {
		// A concurrent identical submit: hand the caller its own subscriber to
		// the live run rather than a second executor.
		return outcome, s.subscribeExisting(snap.RunID), nil
	}
	if snap.State != agentlifecycle.RunQueued {
		// Already executing or terminal: replay nothing, report committed state.
		return outcome, s.replayTicket(snap), nil
	}

	coordCtx, cancelCoord, okReg := s.coord.Register(snap.RunID)
	// Register may refuse during shutdown or a duplicate live task: the
	// context arrives cancelled and the run journals as canceled.
	if !okReg {
		return outcome, nil, ErrRuntimeShuttingDown
	}
	// The bounded wall clock wraps the coordinator context; both cancels run.
	var cancelOnce sync.Once
	runCtx, cancelTimeout := context.WithTimeout(coordCtx, s.timeout)
	runCancel := func() {
		cancelOnce.Do(func() {
			cancelTimeout()
			cancelCoord()
		})
	}

	sr := &serviceRun{runID: snap.RunID, cancel: runCancel, ch: make(chan RunEvent, 256)}
	s.mu.Lock()
	s.live[snap.RunID] = sr
	s.mu.Unlock()

	if _, _, err := s.journal.Transition(ctx, agentlifecycle.RunTransition{
		RunID: snap.RunID, CauseID: string(graphCauseID()), From: agentlifecycle.RunQueued, To: agentlifecycle.RunRunning,
	}); err != nil {
		// The journal refused the start; the run never begins effects.
		s.mu.Lock()
		delete(s.live, snap.RunID)
		s.mu.Unlock()
		runCancel()
		return outcome, nil, fmt.Errorf("journal start: %w", err)
	}

	ticket := &RunTicket{RunID: snap.RunID, Events: sr.ch, Cancel: func() (stopDelivered bool) {
		s.mu.Lock()
		_, isLive := s.live[snap.RunID]
		s.mu.Unlock()
		if !isLive {
			return false
		}
		runCancel()
		return true
	}}
	go s.execute(sr, runCtx, cmd)
	return outcome, ticket, nil
}

// execute owns the run to its terminal transition. It runs in a runtime-owned
// goroutine: view teardown never touches it.
func (s *RunService) execute(sr *serviceRun, runCtx context.Context, cmd InstructionCommand) {
	var result *execution.Result
	var execErr error
	started := time.Now()

	emit := func(ev RunEvent) {
		ev.RunID = sr.runID
		select {
		case sr.ch <- ev:
		default:
			// Bounded subscriber: token floods drop with a count; committed
			// facts wait for the buffer instead of being discarded.
			if ev.Kind == RunEventToken {
				sr.dropped.Add(1)
				return
			}
			sr.ch <- ev
		}
	}
	emit(RunEvent{Kind: RunEventStarted})

	metadata := cmd.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata = cloneMetadata(metadata)
	metadata["journal_run_id"] = sr.runID
	callback := func(token string) { emit(RunEvent{Kind: RunEventToken, Token: token}) }

	result, execErr = s.exec(runCtx, cmd.Instruction, cmd.TaskType, metadata, callback)

	outcome := RunOutcomeCompleted
	terminal := agentlifecycle.RunCompleted
	reason := ""
	paused := false
	switch {
	case errors.Is(execErr, context.Canceled):
		outcome, terminal = RunOutcomeCanceled, agentlifecycle.RunCanceled
	case execErr != nil:
		outcome, terminal = RunOutcomeFailed, agentlifecycle.RunFailed
		reason = classifyFailure(execErr)
	default:
		// A completed pipeline is not a successful turn: a non-success result
		// is a failed run naming its reason — except awaiting_interaction,
		// which pauses the run at the decision boundary.
		if result != nil && !result.Success {
			if categoryOfResult(result) == "awaiting_interaction" {
				outcome = RunOutcomeCompleted
				paused = true
			} else {
				outcome, terminal = RunOutcomeFailed, agentlifecycle.RunFailed
				reason = outcomeFailureReason(result)
				execErr = errors.New(reason)
			}
		}
	}

	// Terminal and pause evidence commits on a bounded cleanup context: the
	// run context is canceled by definition on the cancel path, and a
	// canceled run without a committed outcome would be a lie.
	journalCtx, journalCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer journalCancel()

	if paused {
		if _, _, err := s.journal.Transition(journalCtx, agentlifecycle.RunTransition{
			RunID: sr.runID, CauseID: string(graphCauseID()),
			From: agentlifecycle.RunRunning, To: agentlifecycle.RunAwaitingInteraction,
			Reason: "awaiting interaction frame answer",
		}); err != nil {
			execErr = errors.Join(execErr, fmt.Errorf("journal pause: %w", err))
			paused = false
			outcome, terminal = RunOutcomeFailed, agentlifecycle.RunFailed
		}
	}
	if !paused {
		for attempts := 0; attempts < 2; attempts++ {
			_, _, err := s.journal.Transition(journalCtx, agentlifecycle.RunTransition{
				RunID: sr.runID, CauseID: string(graphCauseID()),
				From: agentlifecycle.RunRunning, To: terminal, Reason: reason,
			})
			if err == nil {
				break
			}
			if errors.Is(err, agentlifecycle.ErrStaleCursor) {
				// A protective cancel already moved the run; never rewrite it.
				break
			}
			// The cancel path transitions from cancel_requested; retry from
			// that state keeps the terminal outcome honest.
			if _, _, err2 := s.journal.Transition(journalCtx, agentlifecycle.RunTransition{
				RunID: sr.runID, CauseID: string(graphCauseID()),
				From: agentlifecycle.RunCancelRequested, To: terminal, Reason: reason,
			}); err2 == nil {
				break
			}
		}
	}

	s.mu.Lock()
	delete(s.live, sr.runID)
	s.mu.Unlock()

	fin := RunEvent{Kind: RunEventFinished, Outcome: outcome, Awaiting: paused, Err: execErr, Result: result, Duration: time.Since(started), Dropped: sr.dropped.Load()}
	sr.closed.Do(func() {
		emit(fin)
		close(sr.ch)
	})
}

// CancelRun requests a protective stop for one run. It reports whether the
// stop reached a live executor; the durable terminal outcome is committed by
// the executor itself. Runs in the journal but not live belong to a previous
// process and are reconciliation business, not cancellation business.
func (s *RunService) CancelRun(runID string) (stopDelivered bool, err error) {
	s.mu.Lock()
	sr, ok := s.live[runID]
	s.mu.Unlock()
	if !ok {
		return false, fmt.Errorf("run %s is not live in this process", runID)
	}
	sr.cancel()
	return true, nil
}

// LiveRunsMap returns the live-run set for membership checks.
func (s *RunService) LiveRunsMap() map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]struct{}, len(s.live))
	for id := range s.live {
		out[id] = struct{}{}
	}
	return out
}

// LiveRuns lists run IDs currently executing in this process.
func (s *RunService) LiveRuns() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.live))
	for id := range s.live {
		ids = append(ids, id)
	}
	return ids
}

// ReconcileAbandoned marks nonterminal journal runs interrupted after a crash.
func (s *RunService) ReconcileAbandoned(ctx context.Context) ([]string, error) {
	return s.journal.ReconcileAbandoned(ctx, string(s.journal.workspace))
}

// Journal exposes the durable journal for history and retention consumers.
func (s *RunService) Journal() *RunJournal { return s.journal }

func (s *RunService) subscribeExisting(runID string) *RunTicket {
	s.mu.Lock()
	sr := s.live[runID]
	s.mu.Unlock()
	if sr == nil {
		return &RunTicket{RunID: runID, Events: closedEventCh(), Cancel: func() bool { return false }}
	}
	// A second subscriber shares the live channel; the exactly-once finished
	// event closes it for both.
	cancelShared := sr.cancel
	return &RunTicket{RunID: runID, Events: sr.ch, Cancel: func() (stopDelivered bool) {
		s.mu.Lock()
		_, isLive := s.live[runID]
		s.mu.Unlock()
		if !isLive {
			return false
		}
		cancelShared()
		return true
	}}
}

func (s *RunService) replayTicket(snap agentlifecycle.RunSnapshot) *RunTicket {
	outcome := RunOutcomeFailed
	switch snap.State {
	case agentlifecycle.RunCompleted, agentlifecycle.RunAwaitingInteraction:
		outcome = RunOutcomeCompleted
	case agentlifecycle.RunCanceled:
		outcome = RunOutcomeCanceled
	case agentlifecycle.RunInterrupted:
		outcome = RunOutcomeInterrupted
	}
	ch := make(chan RunEvent, 1)
	ev := RunEvent{RunID: snap.RunID, Kind: RunEventFinished, Outcome: outcome}
	if snap.State == agentlifecycle.RunAwaitingInteraction || snap.State == agentlifecycle.RunRunning || snap.State == agentlifecycle.RunQueued {
		ev.Awaiting = true
		ev.Outcome = RunOutcomeCompleted
	}
	ch <- ev
	close(ch)
	return &RunTicket{RunID: snap.RunID, Events: ch, Cancel: func() bool { return false }}
}

func nilToCancel(sr *serviceRun) func() bool {
	if sr.cancel == nil {
		return func() bool { return false }
	}
	return func() bool { sr.cancel(); return true }
}

func closedEventCh() <-chan RunEvent {
	ch := make(chan RunEvent)
	close(ch)
	return ch
}

// graphCauseID generates a fresh journal cause identity.
func graphCauseID() string { return graphdb.GenerateID("cause") }

func cloneMetadata(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func classifyFailure(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 256 {
		msg = msg[:256]
	}
	return msg
}

func categoryOfResult(result *execution.Result) string {
	fields := execution.ResultFields(result.Data)
	category, _ := fields["outcome_category"].(string)
	return strings.TrimSpace(category)
}

func outcomeFailureReason(result *execution.Result) string {
	fields := execution.ResultFields(result.Data)
	reason, _ := fields["outcome_reason"].(string)
	category, _ := fields["outcome_category"].(string)
	if strings.TrimSpace(reason) == "" {
		reason = strings.TrimSpace(result.Error)
	}
	if strings.TrimSpace(reason) == "" {
		if category != "" {
			reason = "outcome: " + category
		} else {
			reason = "execution did not succeed"
		}
	}
	return reason
}
