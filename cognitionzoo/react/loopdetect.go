package react

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/governance/bounded"
)

const (
	// loopRingSize is the fixed action window the doom-loop detector keeps.
	// Detection is independent of observation-history trimming: the ring is
	// the detector's own state, not derived from react.tool_observations.
	loopRingSize = 12
	// loopStallThreshold is the same-signature count within the window that
	// signals a stall.
	loopStallThreshold = 4

	loopStateKey    = "react.loop_state"
	loopGuidanceKey = "react.loop_guidance"
)

// actionSig identifies an action in intent space: tool identity plus
// canonicalized arguments plus phase. Observation output is deliberately
// excluded so varying-output loops still match (audit D7); tool identity
// alone is insufficient — identical tool with different arguments is a
// different action.
type actionSig string

// loopDetector is the ReAct doom-loop detector: a fixed loopRingSize-action
// ring of action signatures with per-signature counters. The same signature
// occurring loopStallThreshold times within the window signals a stall; any
// intervening different action dilutes the window. Memory is O(1): the ring
// holds at most loopRingSize entries and the counters map never exceeds ring
// size. The zero value is not usable — construct with newLoopDetector.
type loopDetector struct {
	ring     *bounded.Ring[actionSig]
	counts   map[actionSig]int
	recorded int // highest observation Seq fed to the detector
}

func newLoopDetector() *loopDetector {
	return &loopDetector{
		ring:   bounded.NewRing[actionSig](loopRingSize),
		counts: make(map[actionSig]int, loopRingSize),
	}
}

// record appends an action signature, evicting and decrementing the oldest
// entry when the ring is full, and reports whether that signature has now
// reached the stall threshold within the window.
func (d *loopDetector) record(toolID string, args json.RawMessage, phase string) (stall bool, sig actionSig) {
	sig = actionSignature(toolID, args, phase)
	if snapshot := d.ring.Snapshot(); len(snapshot) == loopRingSize {
		oldest := snapshot[0]
		d.counts[oldest]--
		if d.counts[oldest] <= 0 {
			delete(d.counts, oldest)
		}
	}
	d.ring.Append(sig)
	d.counts[sig]++
	return d.counts[sig] >= loopStallThreshold, sig
}

// count reports the current window count of a signature.
func (d *loopDetector) count(sig actionSig) int { return d.counts[sig] }

// loopStateSnapshot is the envelope-persisted form of the detector state
// (key react.loop_state). It is opaque to every component except the
// detector: only observe writes it, via saveLoopState.
type loopStateSnapshot struct {
	Sigs     []string       `json:"sigs"`
	Counts   map[string]int `json:"counts"`
	Recorded int            `json:"recorded"`
}

func loadLoopDetector(env *contextdata.Envelope) *loopDetector {
	detector := newLoopDetector()
	if env == nil {
		return detector
	}
	snap, ok := contextdata.GetTyped[loopStateSnapshot](env, loopStateKey)
	if !ok {
		return detector
	}
	for _, sig := range snap.Sigs {
		detector.ring.Append(actionSig(sig))
	}
	for sig, count := range snap.Counts {
		detector.counts[actionSig(sig)] = count
	}
	detector.recorded = snap.Recorded
	return detector
}

func saveLoopState(env *contextdata.Envelope, detector *loopDetector) {
	if env == nil {
		return
	}
	snapshot := detector.ring.Snapshot()
	sigs := make([]string, len(snapshot))
	for i, sig := range snapshot {
		sigs[i] = string(sig)
	}
	counts := make(map[string]int, len(detector.counts))
	for sig, count := range detector.counts {
		counts[string(sig)] = count
	}
	contextdata.SetTyped(env, loopStateKey, loopStateSnapshot{Sigs: sigs, Counts: counts, Recorded: detector.recorded})
}

// actionSignature hashes toolID ‖ 0x00 ‖ canonicalJSON(args) ‖ 0x00 ‖ phase.
func actionSignature(toolID string, args json.RawMessage, phase string) actionSig {
	h := sha256.New()
	h.Write([]byte(toolID))
	h.Write([]byte{0x00})
	h.Write([]byte(canonicalJSON(args)))
	h.Write([]byte{0x00})
	h.Write([]byte(phase))
	return actionSig(hex.EncodeToString(h.Sum(nil)))
}

// observationActionSig is the intent-space signature of a recorded tool
// observation.
func observationActionSig(observation ToolObservation) actionSig {
	raw, err := json.Marshal(observation.Args)
	if err != nil {
		raw = json.RawMessage("{}")
	}
	return actionSignature(observation.Tool, raw, observation.Phase)
}

// canonicalJSON normalizes raw JSON arguments: object keys sorted,
// whitespace collapsed, escape forms normalized. Non-JSON input hashes as
// its trimmed text so the signature stays total.
func canonicalJSON(args json.RawMessage) string {
	trimmed := strings.TrimSpace(string(args))
	if trimmed == "" {
		return "{}"
	}
	var decoded any
	if err := json.Unmarshal(args, &decoded); err != nil {
		return trimmed
	}
	normalized, err := json.Marshal(decoded) // Go marshals map keys sorted, compact
	if err != nil {
		return trimmed
	}
	return string(normalized)
}
