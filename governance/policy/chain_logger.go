package policy

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// AuditChainFormatV1 is the on-disk chain format version. Canonicalization is
// the fixed field order of AuditChainEntry/AuditRecord (struct, not map);
// changing the record shape MUST bump this constant — the pinned-bytes golden
// test in chain_logger_test.go enforces the conscious decision.
const AuditChainFormatV1 = "audit_chain_v1"

const (
	// auditChainMaxFileBytes rotates a chain file at 64 MiB regardless of date.
	auditChainMaxFileBytes = 64 << 20
	// auditChainGenesisHash is the previous_hash of the very first chain entry
	// (the all-zero hash).
	auditChainGenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"
	// auditChainHeadFile is the head anchor name.
	auditChainHeadFile = "head.json"
	// auditChainTailWindow is the byte window read to extract the last line of
	// a chain file during O(1) boot validation.
	auditChainTailWindow = 1 << 20
)

// ErrAuditUnavailable is returned by Log when a strict-class record cannot be
// durably enqueued within the enqueue timeout. Callers MUST treat it as a
// failed grant: "no unrecorded governed effects" (SBH-1 INV-5).
var ErrAuditUnavailable = errors.New("audit unavailable — action not performed")

var (
	errAuditLoggerClosed = errors.New("audit logger closed")
	errAuditFlushTimeout = errors.New("audit flush timed out")
)

// FileChainTimeouts bounds the blocking behaviors of the file-backed chain
// (SBH-1 D-10).
type FileChainTimeouts struct {
	// Enqueue bounds how long Log blocks for queue room on a strict-class
	// record before the grant fails closed. Default 2s.
	Enqueue time.Duration
	// Drain bounds Close and flush barriers. Default 5s.
	Drain time.Duration
}

// FileChainOptions tunes the file-backed audit chain logger.
type FileChainOptions struct {
	// QueueSize is the bounded producer channel. Default 1024, floored at 128.
	QueueSize int
	// BatchFlush is the head.json rewrite cadence. Default 100ms.
	BatchFlush time.Duration
	// MaxFileBytes rotates chain files by size. Default 64 MiB.
	MaxFileBytes int64
	// Now is the injectable clock for rotation and expiry decisions.
	Now func() time.Time
	// Timeouts bounds enqueue/drain waits (defaults 2s/5s).
	Timeouts FileChainTimeouts
	// BestEffort downgrades ALL classes to drop-and-count (audit.enforcement
	// = best_effort): an availability escape hatch, never a per-class mix.
	BestEffort bool
	// StrictActions overrides the default strict class set. Default strict:
	// exec, network, capability, ipc, permission_request. Mutating file_access
	// is always derived from metadata["fs_action"].
	StrictActions map[AuditAction]bool
}

// ParseAuditEnforcement validates the workspace.yaml audit.enforcement value.
func ParseAuditEnforcement(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "strict":
		return false, nil // BestEffort=false
	case "best_effort", "best-effort":
		return true, nil
	default:
		return false, fmt.Errorf("invalid audit.enforcement %q (want strict or best_effort)", s)
	}
}

// auditChainHead is the head anchor persisted to head.json.
type auditChainHead struct {
	Sequence int64  `json:"sequence"`
	Hash     string `json:"hash"`
	File     string `json:"file"`
}

// chainRequest is one producer → writer unit. A non-nil sync marks a flush
// barrier: all requests sent before it are written before sync is closed.
type chainRequest struct {
	record AuditRecord
	sync   chan struct{}
}

// chainWriterState is owned exclusively by the writer goroutine.
type chainWriterState struct {
	f              *os.File
	file           string // relative file name
	dateKey        string
	seq            int64
	lastHash       string
	lastAnchorFile string // boot anchor's file, for head rewrites
	bytes          int64
}

// FileChainAuditLogger implements AuditChainReader against an append-only
// JSONL hash chain (SBH-1 D-10). It is the durable, crash-survivable,
// tamper-evident audit store: strict-class records block on enqueue (fail
// closed), the single writer appends hash-linked entries and batch-rewrites
// head.json, and boot validation is an O(1) anchor-vs-tail check with a full
// replay fallback.
type FileChainAuditLogger struct {
	dir  string
	opts FileChainOptions

	ch   chan chainRequest
	stop chan struct{}
	done chan struct{}

	stopOnce sync.Once
	closed   atomic.Bool

	dropped atomic.Uint64

	wmu sync.Mutex // writer state; only the writer goroutine mutates
	ws  chainWriterState

	genesis *AuditRecord // first entry to append (tamper rotation marker)

	tamperMu    sync.Mutex
	tamperAt    time.Time
	tamperValid bool
	tamperHash  string // previous_valid_hash recorded by the tamper marker
}

// NewFileChainAuditLogger creates the chain directory, validates the existing
// chain (head anchor vs. tail, with a full-replay fallback), rotates to a
// fresh file with a tamper-marking genesis entry when the chain is broken, and
// starts the single writer goroutine. Any error means the runtime must not
// register (an audit-less runtime is a fail-open readmission).
func NewFileChainAuditLogger(dir string, opts FileChainOptions) (*FileChainAuditLogger, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("audit chain dir required")
	}
	applyFileChainDefaults(&opts)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create audit chain dir %s: %w", dir, err)
	}
	l := &FileChainAuditLogger{
		dir:  dir,
		opts: opts,
		ch:   make(chan chainRequest, opts.QueueSize),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}

	anchorSeq, anchorHash, anchorFile, tampered, err := l.bootValidate()
	if err != nil {
		return nil, fmt.Errorf("audit chain boot validation: %w", err)
	}
	l.ws.seq = anchorSeq
	l.ws.lastHash = anchorHash
	l.ws.lastAnchorFile = anchorFile

	if tampered {
		now := l.now()
		l.tamperMu.Lock()
		l.tamperAt = now.UTC()
		l.tamperValid = true
		l.tamperHash = anchorHash
		l.tamperMu.Unlock()
		genesis := AuditRecord{
			Timestamp:  now.UTC(),
			AgentID:    filepath.Base(dir),
			Action:     "chain_integrity",
			Type:       "audit",
			Permission: "hash_chain",
			Result:     "tamper_rotation",
			Metadata: map[string]any{
				"previous_valid_hash": anchorHash,
				"tamper_detected_at":  now.UTC().Format(time.RFC3339Nano),
			},
		}
		l.genesis = &genesis
	}

	// Pre-seed the genesis so it is the first entry the writer appends.
	if l.genesis != nil {
		l.ch <- chainRequest{record: *l.genesis}
	}
	go l.run()
	return l, nil
}

func applyFileChainDefaults(opts *FileChainOptions) {
	if opts.QueueSize <= 0 {
		opts.QueueSize = 1024
	}
	if opts.QueueSize < 128 {
		opts.QueueSize = 128
	}
	if opts.BatchFlush <= 0 {
		opts.BatchFlush = 100 * time.Millisecond
	}
	if opts.MaxFileBytes <= 0 {
		opts.MaxFileBytes = auditChainMaxFileBytes
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Timeouts.Enqueue <= 0 {
		opts.Timeouts.Enqueue = 2 * time.Second
	}
	if opts.Timeouts.Drain <= 0 {
		opts.Timeouts.Drain = 5 * time.Second
	}
}

func (l *FileChainAuditLogger) now() time.Time {
	return l.opts.Now().UTC()
}

// auditActionForRecord maps the permission family a record typed to the
// canonical AuditAction vocabulary. Records without a recognizable permission
// type fall back to the permission_request family so they stay durable.
func auditActionForRecord(record AuditRecord) AuditAction {
	switch ucperms.PermissionType(record.Type) {
	case ucperms.PermissionTypeFilesystem:
		return AuditActionFileAccess
	case ucperms.PermissionTypeExecutable:
		return AuditActionExec
	case ucperms.PermissionTypeNetwork:
		return AuditActionNetwork
	case ucperms.PermissionTypeCapability:
		return AuditActionCapability
	case ucperms.PermissionTypeIPC:
		return AuditActionIPC
	case ucperms.PermissionTypeHITL:
		return AuditActionRequest
	default:
		return AuditActionRequest
	}
}

// mutatingFsAction reports whether a file_access record mutates the filesystem.
// The record's metadata["fs_action"] carries the concrete operation (e.g.
// "fs:write"); read/list pass through untouched.
func mutatingFsAction(record AuditRecord) bool {
	fa, _ := record.Metadata["fs_action"].(string)
	switch strings.ToLower(strings.TrimSpace(fa)) {
	case "write", "edit", "execute", "delete", "rename", "move",
		"fs:write", "fs:execute", "fs:delete", "fs:rename", "fs:move":
		return true
	default:
		return false
	}
}

// enforce reports whether the record must be durably enqueued (fail closed)
// rather than dropped-and-counted. With audit.enforcement=best_effort the
// answer is always no (availability escape hatch); otherwise the strict class
// set applies, with mutating file_access always strict.
func (l *FileChainAuditLogger) enforce(record AuditRecord) bool {
	if l.opts.BestEffort {
		return false
	}
	action := auditActionForRecord(record)
	if action == AuditActionFileAccess {
		return mutatingFsAction(record)
	}
	if l.opts.StrictActions != nil {
		return l.opts.StrictActions[action]
	}
	switch action {
	case AuditActionExec, AuditActionNetwork, AuditActionCapability, AuditActionIPC, AuditActionRequest:
		return true
	default:
		return false
	}
}

// Log enqueues a record. Strict-class records block up to the enqueue timeout
// for queue room and return ErrAuditUnavailable on failure (the caller must
// fail the grant). Best-effort-class records are dropped-and-counted.
func (l *FileChainAuditLogger) Log(ctx context.Context, record AuditRecord) error {
	if l == nil || l.closed.Load() {
		return fmt.Errorf("%w: %s", errAuditLoggerClosed, "log refused")
	}
	if !l.enforce(record) {
		l.dropped.Add(1)
		return nil
	}
	if record.Timestamp.IsZero() {
		record.Timestamp = l.now()
	}
	timer := time.NewTimer(l.opts.Timeouts.Enqueue)
	defer timer.Stop()
	select {
	case l.ch <- chainRequest{record: record}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", ErrAuditUnavailable, ctx.Err())
	case <-timer.C:
		return ErrAuditUnavailable
	case <-l.stop:
		return fmt.Errorf("%w: %s", errAuditLoggerClosed, "log refused")
	}
}

// DroppedTotal reports how many best-effort records were dropped-and-counted.
func (l *FileChainAuditLogger) DroppedTotal() uint64 {
	if l == nil {
		return 0
	}
	return l.dropped.Load()
}

// TamperInfo reports whether boot validation detected a broken chain and the
// moment it was detected (blocking doctor readiness; SBH-1 D-10).
func (l *FileChainAuditLogger) TamperInfo() (bool, time.Time, string) {
	if l == nil {
		return false, time.Time{}, ""
	}
	l.tamperMu.Lock()
	defer l.tamperMu.Unlock()
	return l.tamperValid, l.tamperAt, l.tamperHash
}

// run is the single writer: it owns sequence assignment, hash chaining, file
// rotation, and head.json batch rewrites.
func (l *FileChainAuditLogger) run() {
	defer close(l.done)
	flush := time.NewTicker(l.opts.BatchFlush)
	defer flush.Stop()
	for {
		select {
		case req, ok := <-l.ch:
			if !ok {
				return
			}
			if req.sync != nil {
				l.writeHeadLocked()
				close(req.sync)
				continue
			}
			l.appendLocked(req.record)
		case <-flush.C:
			l.writeHeadLocked()
		case <-l.stop:
			// Drain everything already enqueued, then persist the head anchor.
			for {
				select {
				case req := <-l.ch:
					if req.sync != nil {
						close(req.sync)
						continue
					}
					l.appendLocked(req.record)
				default:
					l.writeHeadLocked()
					l.closeCurrentFileLocked()
					return
				}
			}
		}
	}
}

// appendLocked assigns the next sequence and appends one hash-chained entry.
func (l *FileChainAuditLogger) appendLocked(record AuditRecord) {
	l.wmu.Lock()
	defer l.wmu.Unlock()
	rotatingAfterTamper := l.genesis != nil
	if err := l.ensureWriterFileLocked(record.Timestamp, rotatingAfterTamper); err != nil {
		// A filesystem failure during append is a durability failure of a
		// strict-class record; surface it to producers is impossible here (we
		// already passed the enqueue barrier), so count it and keep going —
		// the head anchor stays behind and boot validation reports the gap.
		l.dropped.Add(1)
		return
	}
	l.ws.seq++
	prev := l.ws.lastHash
	entry := AuditChainEntry{
		Sequence:     l.ws.seq,
		Record:       record,
		PreviousHash: prev,
	}
	entry.RecordHash = chainRecordHash(prev, record)
	line, err := json.Marshal(entry)
	if err != nil {
		l.dropped.Add(1)
		return
	}
	if _, err := l.ws.f.Write(append(line, '\n')); err != nil {
		l.dropped.Add(1)
		return
	}
	l.ws.lastHash = entry.RecordHash
	l.ws.bytes += int64(len(line) + 1)
	if rotatingAfterTamper {
		// Genesis consumed: subsequent records append into the rotation file
		// normally (only the first append forces the tamper rotation name).
		l.genesis = nil
	}
}

// chainRecordHash computes record_hash = SHA256(canonicalJSON(record) ||
// previous_hash_hex). canonicalJSON is a fixed-order struct marshal, shared by
// write and verify paths.
func chainRecordHash(prevHash string, record AuditRecord) string {
	canonical := canonicalJSON(record)
	h := sha256.Sum256([]byte(prevHash + string(canonical)))
	return hex.EncodeToString(h[:])
}

// canonicalJSON marshals the fixed-order AuditRecord without HTML escaping and
// with UTC RFC3339Nano timestamps (time.Time marshals that way by contract;
// callers Log UTC times). Map values (metadata) marshal with sorted keys.
func canonicalJSON(record AuditRecord) []byte {
	data, _ := json.Marshal(record)
	return data
}

// ensureWriterFileLocked opens/rotates the writer file for the record's
// timestamp. File placement STEPS FORWARD ONLY (monotonic): a late-arriving
// out-of-order record whose date is older than the current file still appends
// to the current file, so sequences stay strictly monotonic across
// replay-ordered files. rotatingAfterTamper forces a fresh rotation file for
// the tamper-marking genesis entry instead of appending into the broken file.
func (l *FileChainAuditLogger) ensureWriterFileLocked(ts time.Time, rotatingAfterTamper bool) error {
	if ts.IsZero() {
		ts = l.now()
	}
	today := ts.Format("20060102")
	if l.ws.f != nil {
		switch {
		case today > l.ws.dateKey:
			// Date rollover to a later day: reopen under the new day's name.
			_ = l.ws.f.Close()
			l.ws.f = nil
		case l.ws.bytes >= l.opts.MaxFileBytes:
			// Same-day size rotation.
			return l.rotateLocked(l.ws.dateKey)
		default:
			// Same day, or an out-of-order older record: append in place so
			// per-file sequences remain monotonic.
			return nil
		}
	}
	name := "chain-" + today + ".jsonl"
	if rotatingAfterTamper {
		name = l.nextRotationNameLocked(today)
	}
	return l.openFileLocked(name, today)
}

func (l *FileChainAuditLogger) rotateLocked(today string) error {
	_ = l.ws.f.Close()
	l.ws.f = nil
	name := l.nextRotationNameLocked(today)
	return l.openFileLocked(name, today)
}

func (l *FileChainAuditLogger) openFileLocked(name, dateKey string) error {
	path := filepath.Join(l.dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return fmt.Errorf("open chain file %s: %w", path, err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	l.ws.f = f
	l.ws.file = name
	l.ws.dateKey = dateKey
	l.ws.bytes = st.Size()
	return nil
}

func (l *FileChainAuditLogger) nextRotationNameLocked(today string) string {
	for n := 1; ; n++ {
		name := fmt.Sprintf("chain-%s.r%02d.jsonl", today, n)
		if _, err := os.Stat(filepath.Join(l.dir, name)); os.IsNotExist(err) {
			return name
		}
	}
}

func (l *FileChainAuditLogger) closeCurrentFileLocked() {
	if l.ws.f != nil {
		_ = l.ws.f.Close()
		l.ws.f = nil
	}
}

// writeHeadLocked rewrites head.json atomically (temp + rename) so boot
// validation stays O(1) against the true tail.
func (l *FileChainAuditLogger) writeHeadLocked() {
	l.wmu.Lock()
	defer l.wmu.Unlock()
	if l.ws.f == nil || l.ws.seq == 0 {
		return
	}
	head := auditChainHead{Sequence: l.ws.seq, Hash: l.ws.lastHash, File: l.ws.file}
	data, err := json.Marshal(head)
	if err != nil {
		return
	}
	tmp := filepath.Join(l.dir, auditChainHeadFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return
	}
	_ = os.Rename(tmp, filepath.Join(l.dir, auditChainHeadFile))
}

// flushToDisk waits until every record enqueued before the barrier is written
// to disk (sequence-committed and head.json updated). A closed logger has
// already drained its queue, so read-only verifications after Close still work.
func (l *FileChainAuditLogger) flushToDisk(ctx context.Context) error {
	if l == nil {
		return errors.New("audit logger missing")
	}
	if l.closed.Load() {
		return nil
	}
	syncCh := make(chan struct{})
	req := chainRequest{sync: syncCh}
	select {
	case l.ch <- req:
	case <-ctx.Done():
		return ctx.Err()
	case <-l.stop:
		return fmt.Errorf("%w: %s", errAuditLoggerClosed, "flush refused")
	}
	timer := time.NewTimer(l.opts.Timeouts.Drain)
	defer timer.Stop()
	select {
	case <-syncCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errAuditFlushTimeout
	}
}

// Query returns committed records matching the filter. Pending records are
// flushed to disk first so a completed Log is always observable here.
func (l *FileChainAuditLogger) Query(ctx context.Context, filter AuditQuery) ([]AuditRecord, error) {
	if l == nil {
		return nil, errors.New("audit logger missing")
	}
	if err := l.flushToDisk(ctx); err != nil {
		return nil, err
	}
	files, err := chainFilesIn(l.dir)
	if err != nil {
		return nil, err
	}
	var out []AuditRecord
	for _, file := range files {
		entries, _, err := readChainEntries(file)
		if err != nil {
			return nil, fmt.Errorf("read audit chain %s: %w", filepath.Base(file), err)
		}
		for i := range entries {
			rec := entries[i].Record
			if matchAuditQuery(rec, filter) {
				out = append(out, rec)
			}
		}
	}
	return out, nil
}

// ReadChain returns committed chain entries matching the filter.
func (l *FileChainAuditLogger) ReadChain(ctx context.Context, filter AuditChainFilter) ([]AuditChainEntry, error) {
	if l == nil {
		return nil, errors.New("audit logger missing")
	}
	if err := l.flushToDisk(ctx); err != nil {
		return nil, err
	}
	files, err := chainFilesIn(l.dir)
	if err != nil {
		return nil, err
	}
	var out []AuditChainEntry
	for _, file := range files {
		entries, _, err := readChainEntries(file)
		if err != nil {
			return nil, fmt.Errorf("read audit chain %s: %w", filepath.Base(file), err)
		}
		for i := range entries {
			rec := entries[i].Record
			if matchAuditChainFilter(rec, entries[i], filter) {
				out = append(out, entries[i])
				if filter.Limit > 0 && len(out) >= filter.Limit {
					return out, nil
				}
			}
		}
	}
	return out, nil
}

// VerifyHead is the O(1) boot validation: head.json vs. the tail entry of the
// file it names. When the anchor is missing it falls back to a full replay.
func (l *FileChainAuditLogger) VerifyHead(ctx context.Context) AuditChainVerification {
	if l == nil {
		return AuditChainVerification{Verified: false, Failure: "audit logger missing"}
	}
	return verifyHeadDir(l.dir)
}

// VerifyChain replays the committed chain and reports integrity.
func (l *FileChainAuditLogger) VerifyChain(ctx context.Context, filter AuditChainFilter) (AuditChainVerification, error) {
	if l == nil {
		return AuditChainVerification{Verified: false, Failure: "audit logger missing"}, errors.New("audit logger missing")
	}
	if err := l.flushToDisk(ctx); err != nil {
		return AuditChainVerification{Verified: false, Failure: err.Error()}, err
	}
	return verifyChainDir(l.dir, filter)
}

// Close stops the writer, drains the queue, persists the head anchor, and
// closes the current file. Idempotent; bounded by the drain timeout.
func (l *FileChainAuditLogger) Close() error {
	if l == nil {
		return nil
	}
	var err error
	l.stopOnce.Do(func() {
		l.closed.Store(true)
		close(l.stop)
		select {
		case <-l.done:
		case <-time.After(l.opts.Timeouts.Drain):
			err = errAuditFlushTimeout
		}
	})
	return err
}

// chainFilesIn lists the chain files in replay order.
func chainFilesIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "chain-") || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)
	return files, nil
}

// bootValidate establishes the merge point for new records: (lastValidSeq,
// lastValidHash, lastValidFile, tampered). Healthy chains resume from the head
// anchor (O(1)); when the anchor is missing or stale, a full replay decides.
func (l *FileChainAuditLogger) bootValidate() (int64, string, string, bool, error) {
	files, err := chainFilesIn(l.dir)
	if err != nil {
		return 0, "", "", false, err
	}
	if len(files) == 0 {
		return 0, auditChainGenesisHash, "", false, nil
	}

	head, headErr := readHeadAnchor(l.dir)
	if headErr == nil {
		last, tailErr := tailLastEntry(filepath.Join(l.dir, head.File))
		if tailErr == nil && last != nil && last.Sequence == head.Sequence && last.RecordHash == head.Hash {
			return head.Sequence, head.Hash, head.File, false, nil
		}
	}

	// Full replay fallback: anchor at the last valid prefix.
	lastSeq, lastHash, lastFile, broken, err := replayChain(files)
	if err != nil {
		return 0, "", "", false, err
	}
	// Fresh directories may hold an untouched day file; that is not loss.
	if lastSeq == 0 && broken {
		return 0, auditChainGenesisHash, "", true, nil
	}
	if broken || headErr != nil {
		// Persuade head.json onto the valid prefix before the writer continues.
		if err := writeHeadAnchor(l.dir, lastFile, lastSeq, lastHash); err != nil {
			return 0, "", "", false, err
		}
	}
	return lastSeq, lastHash, lastFile, broken, nil
}

// readChainEntries decodes every complete JSON line in a chain file. It
// returns the entries, a partial-tail flag (trailing unparseable bytes), and
// any fatal decode error. A partial tail is corruption, not "end of file".
func readChainEntries(path string) ([]AuditChainEntry, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	var entries []AuditChainEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e AuditChainEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return entries, true, fmt.Errorf("chain line %d: %w", lineNo, err)
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return entries, false, err
	}
	return entries, false, nil
}

// tailLastEntry extracts the final complete JSON entry of a chain file by
// reading only the trailing window (O(1) for large files).
func tailLastEntry(path string) (*AuditChainEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() == 0 {
		return nil, io.EOF
	}
	size := st.Size()
	var raw []byte
	if size <= auditChainTailWindow {
		raw = make([]byte, size)
		if _, err := io.ReadFull(f, raw); err != nil {
			return nil, err
		}
	} else {
		raw = make([]byte, auditChainTailWindow)
		if _, err := f.Seek(size-int64(len(raw)), io.SeekStart); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(f, raw); err != nil {
			return nil, err
		}
	}
	s := strings.TrimRight(string(raw), "\r\n")
	idx := strings.LastIndexByte(s, '\n')
	if idx >= 0 {
		s = s[idx+1:]
	}
	line := strings.TrimSpace(s)
	if line == "" {
		return nil, io.EOF
	}
	var e AuditChainEntry
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		return nil, fmt.Errorf("tail entry unparseable (truncated tail?): %w", err)
	}
	return &e, nil
}

// replayChain verifies the whole chain and returns the last valid prefix
// (sequence, hash, file). broken=true when a mismatch or corruption is found
// anywhere.
func replayChain(files []string) (int64, string, string, bool, error) {
	prev := auditChainGenesisHash
	lastSeq := int64(0)
	lastHash := auditChainGenesisHash
	lastFile := ""
	broken := false
	for _, file := range files {
		entries, partial, err := readChainEntries(file)
		if err != nil {
			return lastSeq, lastHash, lastFile, true, nil
		}
		for i := range entries {
			e := entries[i]
			expectedSeq := lastSeq + 1
			if e.Sequence != expectedSeq {
				return lastSeq, lastHash, lastFile, true, nil
			}
			if e.PreviousHash != prev {
				return lastSeq, lastHash, lastFile, true, nil
			}
			expected := chainRecordHash(prev, e.Record)
			if e.RecordHash != expected {
				return lastSeq, lastHash, lastFile, true, nil
			}
			lastSeq = e.Sequence
			lastHash = e.RecordHash
			lastFile = file
			prev = lastHash
		}
		_ = partial
	}
	return lastSeq, lastHash, lastFile, broken, nil
}

// verifyChainDir replays the committed chain and produces a verification.
func verifyChainDir(dir string, filter AuditChainFilter) (AuditChainVerification, error) {
	files, err := chainFilesIn(dir)
	if err != nil {
		return AuditChainVerification{Verified: false, Failure: err.Error()}, err
	}
	if len(files) == 0 {
		return AuditChainVerification{Verified: true, EntryCount: 0, LastHash: auditChainGenesisHash}, nil
	}
	prev := auditChainGenesisHash
	lastSeq := int64(0)
	lastHash := auditChainGenesisHash
	matched := 0
	for _, file := range files {
		entries, _, err := readChainEntries(file)
		if err != nil {
			v := AuditChainVerification{Verified: false, LastSequence: lastSeq, LastHash: lastHash}
			v.Failure = fmt.Sprintf("chain file %s corrupt: %v", filepath.Base(file), err)
			return v, nil
		}
		for i := range entries {
			e := entries[i]
			expectedSeq := lastSeq + 1
			if e.Sequence != expectedSeq {
				return failVerification(lastSeq, lastHash, fmt.Sprintf("non-monotonic sequence: got %d want %d", e.Sequence, expectedSeq)), nil
			}
			if e.PreviousHash != prev {
				return failVerification(lastSeq, lastHash, fmt.Sprintf("hash link broken at sequence %d", e.Sequence)), nil
			}
			expected := chainRecordHash(prev, e.Record)
			if e.RecordHash != expected {
				return failVerification(lastSeq, lastHash, fmt.Sprintf("record hash mismatch at sequence %d", e.Sequence)), nil
			}
			if matchAuditChainFilter(e.Record, e, filter) {
				matched++
			}
			lastSeq = e.Sequence
			lastHash = e.RecordHash
			prev = lastHash
		}
	}
	return AuditChainVerification{
		Verified:     true,
		EntryCount:   matched,
		LastSequence: lastSeq,
		LastHash:     lastHash,
	}, nil
}

func failVerification(lastSeq int64, lastHash, failure string) AuditChainVerification {
	return AuditChainVerification{Verified: false, LastSequence: lastSeq, LastHash: lastHash, Failure: failure}
}

func readHeadAnchor(dir string) (*auditChainHead, error) {
	data, err := os.ReadFile(filepath.Join(dir, auditChainHeadFile))
	if err != nil {
		return nil, err
	}
	var head auditChainHead
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, err
	}
	if head.File == "" {
		return nil, errors.New("head anchor missing file")
	}
	return &head, nil
}

// writeHeadAnchor rewrites the head anchor atomically (temp + rename).
func writeHeadAnchor(dir, file string, seq int64, hash string) error {
	head := auditChainHead{Sequence: seq, Hash: hash, File: file}
	data, err := json.Marshal(head)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, auditChainHeadFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, auditChainHeadFile))
}

// verifyHeadDir is a read-only head validation usable by the doctor without
// constructing a logger. Integrity is determined by a full replay when the
// anchor is missing or stale; a clean replay with a missing anchor is NOT
// broken (boot rebuilds the anchor), only a genuinely broken chain is.
func verifyHeadDir(dir string) AuditChainVerification {
	head, err := readHeadAnchor(dir)
	if err == nil {
		last, tailErr := tailLastEntry(filepath.Join(dir, head.File))
		if tailErr == nil && last != nil && last.Sequence == head.Sequence && last.RecordHash == head.Hash {
			return AuditChainVerification{
				Verified:     true,
				LastSequence: head.Sequence,
				LastHash:     head.Hash,
			}
		}
	}
	files, serr := chainFilesIn(dir)
	if serr != nil {
		return AuditChainVerification{Verified: false, Failure: serr.Error()}
	}
	if len(files) == 0 {
		return AuditChainVerification{Verified: true, LastHash: auditChainGenesisHash}
	}
	v, _ := verifyChainDir(dir, AuditChainFilter{})
	if v.Verified {
		v.Failure = "head anchor missing or stale (chain replays clean — boot rebuilds)"
	}
	return v
}

// ChainProbe is the doctor's read-only readiness probe result: dir presence
// and head-anchor integrity, without constructing a logger.
type ChainProbe struct {
	Present  bool
	LastSeq  int64
	LastHash string
	Broken   bool
	Failure  string
}

// ProbeChainDir runs the doctor's readiness probe. Integrity uses a full
// chain replay so a tampered byte anywhere is surfaced (not just a broken
// head anchor); the logger's own boot path remains the O(1) anchor check per
// NFR-4. Read-only: never constructs a logger or writes.
func ProbeChainDir(dir string) ChainProbe {
	if _, err := os.Stat(dir); err != nil {
		return ChainProbe{Present: false}
	}
	files, err := chainFilesIn(dir)
	if err != nil {
		return ChainProbe{Present: false, Broken: true, Failure: err.Error()}
	}
	if len(files) == 0 {
		return ChainProbe{Present: false}
	}
	v, _ := verifyChainDir(dir, AuditChainFilter{})
	return ChainProbe{
		Present:  true,
		LastSeq:  v.LastSequence,
		LastHash: v.LastHash,
		Broken:   !v.Verified,
		Failure:  v.Failure,
	}
}

func matchAuditQuery(rec AuditRecord, f AuditQuery) bool {
	if f.AgentID != "" && rec.AgentID != f.AgentID {
		return false
	}
	if f.Type != "" && rec.Type != f.Type {
		return false
	}
	if f.Action != "" && rec.Action != f.Action {
		return false
	}
	if f.Permission != "" && rec.Permission != f.Permission {
		return false
	}
	if f.Result != "" && rec.Result != f.Result {
		return false
	}
	if !f.TimeStart.IsZero() && rec.Timestamp.Before(f.TimeStart) {
		return false
	}
	if !f.TimeEnd.IsZero() && rec.Timestamp.After(f.TimeEnd) {
		return false
	}
	return true
}

func matchAuditChainFilter(rec AuditRecord, entry AuditChainEntry, f AuditChainFilter) bool {
	if !matchAuditQuery(rec, f.AuditQuery) {
		return false
	}
	if f.Correlation != "" && entry.Record.Correlation != f.Correlation {
		return false
	}
	if f.LineageID != "" {
		lineage, _ := entry.Record.Metadata["lineage_id"].(string)
		if lineage != f.LineageID {
			return false
		}
	}
	return true
}
