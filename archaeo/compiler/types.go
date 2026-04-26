package compiler

import biknowledgecc "codeburg.org/lexbit/relurpify/framework/biknowledgecc"

type ChunkID = biknowledgecc.ChunkID
type EdgeID = biknowledgecc.EdgeID
type FreshnessState = biknowledgecc.FreshnessState
type CompilerPath = biknowledgecc.CompilerPath
type ViewKind = biknowledgecc.ViewKind
type EdgeKind = biknowledgecc.EdgeKind
type ProvenanceSource = biknowledgecc.ProvenanceSource
type ChunkProvenance = biknowledgecc.ChunkProvenance
type ChunkBody = biknowledgecc.ChunkBody
type ChunkView = biknowledgecc.ChunkView
type KnowledgeChunk = biknowledgecc.KnowledgeChunk
type ChunkEdge = biknowledgecc.ChunkEdge
type ChunkStore = biknowledgecc.ChunkStore
type EventKind = biknowledgecc.EventKind
type Event = biknowledgecc.Event
type BootstrapCompletePayload = biknowledgecc.BootstrapCompletePayload
type CodeRevisionChangedPayload = biknowledgecc.CodeRevisionChangedPayload
type ChunkStaledPayload = biknowledgecc.ChunkStaledPayload
type EventBus = biknowledgecc.EventBus
type Module = biknowledgecc.Module

const (
	FreshnessValid      = biknowledgecc.FreshnessValid
	FreshnessStale      = biknowledgecc.FreshnessStale
	FreshnessInvalid    = biknowledgecc.FreshnessInvalid
	FreshnessUnverified = biknowledgecc.FreshnessUnverified

	CompilerDeterministic = biknowledgecc.CompilerDeterministic
	CompilerLLMAssisted   = biknowledgecc.CompilerLLMAssisted
	CompilerUserDirect    = biknowledgecc.CompilerUserDirect

	ViewKindPattern    = biknowledgecc.ViewKindPattern
	ViewKindDecision   = biknowledgecc.ViewKindDecision
	ViewKindConstraint = biknowledgecc.ViewKindConstraint
	ViewKindPlanStep   = biknowledgecc.ViewKindPlanStep
	ViewKindAnchor     = biknowledgecc.ViewKindAnchor
	ViewKindTension    = biknowledgecc.ViewKindTension
	ViewKindIntent     = biknowledgecc.ViewKindIntent

	EdgeKindGrounds            = biknowledgecc.EdgeKindGrounds
	EdgeKindContradicts        = biknowledgecc.EdgeKindContradicts
	EdgeKindRefines            = biknowledgecc.EdgeKindRefines
	EdgeKindGeneralizes        = biknowledgecc.EdgeKindGeneralizes
	EdgeKindExemplifies        = biknowledgecc.EdgeKindExemplifies
	EdgeKindDerivesFrom        = biknowledgecc.EdgeKindDerivesFrom
	EdgeKindComposedOf         = biknowledgecc.EdgeKindComposedOf
	EdgeKindSupersedes         = biknowledgecc.EdgeKindSupersedes
	EdgeKindRequiresContext    = biknowledgecc.EdgeKindRequiresContext
	EdgeKindAmplifies          = biknowledgecc.EdgeKindAmplifies
	EdgeKindInvalidates        = biknowledgecc.EdgeKindInvalidates
	EdgeKindDependsOnCodeState = biknowledgecc.EdgeKindDependsOnCodeState
	EdgeKindConfirmed          = biknowledgecc.EdgeKindConfirmed
	EdgeKindRejected           = biknowledgecc.EdgeKindRejected
	EdgeKindRefinedBy          = biknowledgecc.EdgeKindRefinedBy
	EdgeKindDeferred           = biknowledgecc.EdgeKindDeferred

	EventBootstrapComplete   = biknowledgecc.EventBootstrapComplete
	EventCodeRevisionChanged = biknowledgecc.EventCodeRevisionChanged
	EventChunkStaled         = biknowledgecc.EventChunkStaled
	EventPatternConfirmed    = biknowledgecc.EventPatternConfirmed
	EventAnchorConfirmed     = biknowledgecc.EventAnchorConfirmed
	EventIndexEntryProduced  = biknowledgecc.EventIndexEntryProduced
	EventUserStatement       = biknowledgecc.EventUserStatement
)
