// Package interaction provides the UX-agnostic frame protocol.
//
// Interaction frames are durable, serializable structures that decouple
// Euclo execution from any specific presentation layer.
//
// # Two frame classes (normative, owner ruling R-2 of 2026-10-10)
//
// The system has exactly two frame classes, and they resolve through
// different machinery:
//
//   - Policy frames (on_error: ask, HITL-adjacent): resolved BLOCKING
//     through the Resolver interface in this package — Resolve waits for the
//     answer with exactly-once delivery, an optional deadline, and the
//     Expired outcome. The FrameResolver and the surfaces' Resolver
//     implementations (relurpish TUI, agenttest's permissive resolver)
//     belong to this class.
//
//   - DSL ask frames (thoughtrecipe `ask` steps): resolved through
//     PAUSE/RESUME — the ask node emits the frame, writes
//     euclo.interaction.pause, and the graph short-circuits; the run ends
//     with a paused outcome and the surface answers whenever it likes via
//     the runtime's ResolveInteractionFrame, which writes the answer onto
//     the live envelope and re-enters the subgraph at the stored resume
//     node. DSL ask frames have NO deadline (pause means answer-anytime) and
//     NO resolver involvement.
//
// Ruling R-2 (2026-10-10) superseded wave 3 D10: the pause/resume mechanism
// is the DECIDED semantics for DSL `ask user`; no blocking rewrite and no
// pause-path deletion. The pause key itself (euclo.interaction.pause) is
// production surface and pinned by test.
package interaction
