package memory

type MemoryScope string

const (
	MemoryScopeProject MemoryScope = "project"
	MemoryScopeSession MemoryScope = "session"
	MemoryScopeGlobal  MemoryScope = "global"
)

type KnowledgeKind string

const (
	KnowledgeKindFact     KnowledgeKind = "fact"
	KnowledgeKindDecision KnowledgeKind = "decision"
	KnowledgeKindIssue    KnowledgeKind = "issue"
)

type DeclarativeMemoryKind string

const (
	DeclarativeMemoryKindFact             DeclarativeMemoryKind = "fact"
	DeclarativeMemoryKindDecision         DeclarativeMemoryKind = "decision"
	DeclarativeMemoryKindConstraint       DeclarativeMemoryKind = "constraint"
	DeclarativeMemoryKindPreference       DeclarativeMemoryKind = "preference"
	DeclarativeMemoryKindProjectKnowledge DeclarativeMemoryKind = "project_knowledge"
)

type ProceduralMemoryKind string

const (
	ProceduralMemoryKindRoutine               ProceduralMemoryKind = "routine"
	ProceduralMemoryKindRecoveryRoutine       ProceduralMemoryKind = "recovery_routine"
	ProceduralMemoryKindCapabilityComposition ProceduralMemoryKind = "capability_composition"
)
