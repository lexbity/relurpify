package thoughtrecipe

// SourceLocation identifies a concrete position in a source file.
type SourceLocation struct {
	File   string
	Line   int
	Column int
}

// SourceSpan captures a half-open source range for an AST node.
type SourceSpan struct {
	Start SourceLocation
	End   SourceLocation
}

// NewSpan creates a source span from explicit coordinates.
func NewSpan(file string, startLine, startColumn, endLine, endColumn int) SourceSpan {
	return SourceSpan{
		Start: SourceLocation{File: file, Line: startLine, Column: startColumn},
		End:   SourceLocation{File: file, Line: endLine, Column: endColumn},
	}
}

type positioned struct {
	Span SourceSpan
}

// GetSpan returns the source span for a positioned node.
func (p positioned) GetSpan() SourceSpan {
	return p.Span
}

// Node is the base interface for AST nodes.
type Node interface {
	GetSpan() SourceSpan
}

// Declaration is a top-level thoughtrecipe declaration.
type Declaration interface {
	Node
	declarationNode()
}

// ImportKind identifies a top-level import declaration target kind.
type ImportKind string

const (
	ImportKindPrompt ImportKind = "prompt"
	ImportKindRecipe ImportKind = "recipe"
)

// ValueExpr is a source-oriented value expression.
type ValueExpr interface {
	Node
	valueExprNode()
}

// TypeExpr is a source-oriented type expression.
type TypeExpr interface {
	Node
	typeExprNode()
}

// TypeDefinition is the body of a type declaration.
type TypeDefinition interface {
	Node
	typeDefinitionNode()
}

// ExecutionItem is a clause that can appear in execution-oriented blocks.
type ExecutionItem interface {
	Node
	executionItemNode()
}

// AskItem is a clause that can appear in ask blocks.
type AskItem interface {
	Node
	askItemNode()
}

// ThoughtRecipeDocument is the source-level Euclo thoughtrecipe AST.
type ThoughtRecipeDocument struct {
	SourcePath   string
	Name         string
	Header       ThoughtRecipeHeader
	Declarations []Declaration
}

// ThoughtRecipeHeader captures the thoughtrecipe declaration line and optional description.
type ThoughtRecipeHeader struct {
	positioned
	Name        Identifier
	Description *StringLiteral
}

// TriggerDecl describes the trigger declaration.
type TriggerDecl struct {
	positioned
	Policy       Identifier
	RouteKind    TriggerRouteKind
	Lines        []TriggerLine
	Associations []TriggerAssociationDecl
}

func (TriggerDecl) declarationNode() {}

// TriggerRouteKind identifies the route contract declared by a trigger.
type TriggerRouteKind string

const (
	TriggerRouteKindUnknown    TriggerRouteKind = ""
	TriggerRouteKindCapability TriggerRouteKind = "capability"
	TriggerRouteKindIntent     TriggerRouteKind = "intent"
)

// TriggerLine preserves a single trigger policy line.
type TriggerLine struct {
	positioned
	Effect   Identifier
	Resource ValueExpr
	Raw      string
}

// TriggerAssociationDecl preserves trigger-local family/keyword metadata.
type TriggerAssociationDecl struct {
	positioned
	Name   Identifier
	Values *ListLiteral
	Raw    string
}

// InputDecl describes an input binding.
type InputDecl struct {
	positioned
	Name   Identifier
	Source ValueExpr
}

func (InputDecl) declarationNode() {}

// TypeDecl describes a named type declaration.
type TypeDecl struct {
	positioned
	Name DefinitionName
	Body TypeDefinition
}

func (TypeDecl) declarationNode() {}

// ImportDecl binds a prompt or recipe identifier into the local recipe scope.
type ImportDecl struct {
	positioned
	Kind   ImportKind
	Target PathExpr
	Alias  Identifier
}

func (ImportDecl) declarationNode() {}

// DefinitionName preserves the declared name for a type or agent.
type DefinitionName struct {
	positioned
	Value string
}

// RecordTypeDefinition represents a structural type body.
type RecordTypeDefinition struct {
	positioned
	Fields []TypeField
}

func (RecordTypeDefinition) typeDefinitionNode() {}

// EnumTypeDefinition represents an inline enum type body.
type EnumTypeDefinition struct {
	positioned
	Values []Identifier
}

func (EnumTypeDefinition) typeDefinitionNode() {}

// TypeField describes a single structural field.
type TypeField struct {
	positioned
	Name Identifier
	Type TypeExpr
}

// AgentDecl describes an agent binding.
type AgentDecl struct {
	positioned
	Name      Identifier
	AgentType Identifier
}

func (AgentDecl) declarationNode() {}

// RunDecl describes a run block.
type RunDecl struct {
	positioned
	Agent Identifier
	Items []ExecutionItem
}

func (RunDecl) declarationNode()   {}
func (RunDecl) executionItemNode() {}

// DelegateDecl describes a delegate block.
type DelegateDecl struct {
	positioned
	Agent Identifier
	Items []ExecutionItem
}

func (DelegateDecl) declarationNode()   {}
func (DelegateDecl) executionItemNode() {}

// RouteDecl describes a constrained route block.
type RouteDecl struct {
	positioned
	Branches []RouteBranch
}

func (RouteDecl) declarationNode()   {}
func (RouteDecl) executionItemNode() {}

// AskDecl describes a human-in-the-loop clarification block.
type AskDecl struct {
	positioned
	Items []AskItem
}

func (AskDecl) declarationNode()   {}
func (AskDecl) executionItemNode() {}

// PipelineDecl describes a sequential pipeline block.
type PipelineDecl struct {
	positioned
	Stages []PipelineStage
}

func (PipelineDecl) declarationNode()   {}
func (PipelineDecl) executionItemNode() {}

// Identifier is a bare identifier in source form.
type Identifier struct {
	positioned
	Value string
}

func (Identifier) valueExprNode() {}

// StringLiteral preserves the raw and unescaped string content.
type StringLiteral struct {
	positioned
	Raw   string
	Value string
}

func (StringLiteral) valueExprNode() {}

// PathExpr preserves a dotted path expression.
type PathExpr struct {
	positioned
	Raw   string
	Parts []Identifier
}

func (PathExpr) valueExprNode() {}

// ListLiteral preserves a literal list expression.
type ListLiteral struct {
	positioned
	Raw     string
	Entries []ValueExpr
}

func (ListLiteral) valueExprNode() {}

// NumberLiteral preserves numeric and percent literals.
type NumberLiteral struct {
	positioned
	Raw   string
	Value string
}

func (NumberLiteral) valueExprNode() {}

// NamedTypeExpr is a type reference by name.
type NamedTypeExpr struct {
	positioned
	Name PathExpr
}

func (NamedTypeExpr) typeExprNode() {}

// ListTypeExpr describes list<T>.
type ListTypeExpr struct {
	positioned
	Element TypeExpr
}

func (ListTypeExpr) typeExprNode() {}

// OptionalTypeExpr describes optional<T>.
type OptionalTypeExpr struct {
	positioned
	Element TypeExpr
}

func (OptionalTypeExpr) typeExprNode() {}

// MapTypeExpr describes map<K, V>.
type MapTypeExpr struct {
	positioned
	Key   TypeExpr
	Value TypeExpr
}

func (MapTypeExpr) typeExprNode() {}

// UnionTypeExpr describes a|b|c.
type UnionTypeExpr struct {
	positioned
	Options []TypeExpr
}

func (UnionTypeExpr) typeExprNode() {}

// PredicateExpr preserves a constrained routing predicate fragment.
type PredicateExpr struct {
	positioned
	Raw      string
	Kind     string
	Subject  PathExpr
	Operator string
	Value    ValueExpr
}

// FromClause provides a source value for execution.
type FromClause struct {
	positioned
	Source ValueExpr
}

func (FromClause) executionItemNode() {}

// GoalClause provides a goal string for execution.
type GoalClause struct {
	positioned
	Text     StringLiteral
	PromptID *PromptRef
}

func (GoalClause) executionItemNode() {}

// DirectiveClause preserves an agent-specific directive line.
type DirectiveClause struct {
	positioned
	Name      Identifier
	Arguments []ValueExpr
	Raw       string
}

func (DirectiveClause) executionItemNode() {}

// DirectiveBlock preserves a nested directive body.
type DirectiveBlock struct {
	positioned
	Name      Identifier
	Arguments []ValueExpr
	Predicate *PredicateExpr
	Raw       string
	Body      []ExecutionItem
}

func (DirectiveBlock) executionItemNode() {}

// CapabilityInvocation preserves a do relurpic:<capability> clause.
type CapabilityInvocation struct {
	positioned
	Namespace  Identifier
	Capability Identifier
	Target     ValueExpr
	Input      ValueExpr
}

func (CapabilityInvocation) executionItemNode() {}

// CaptureBlock preserves a capture block with typed or untyped bindings.
type CaptureBlock struct {
	positioned
	Inline   bool
	Bindings []CaptureBinding
}

func (CaptureBlock) executionItemNode() {}
func (CaptureBlock) askItemNode()       {}

// CaptureBinding preserves a single capture binding.
type CaptureBinding struct {
	positioned
	Source      ValueExpr
	Annotation  TypeExpr
	Destination PathExpr
	Forwarding  bool
}

// QuestionClause preserves ask user question text.
type QuestionClause struct {
	positioned
	Text     StringLiteral
	PromptID *PromptRef
}

func (QuestionClause) askItemNode() {}

// PromptRef references a locally imported prompt binding.
type PromptRef struct {
	positioned
	Name       Identifier
	ResolvedID string
}

// ChoicesListClause preserves an inline choice list.
type ChoicesListClause struct {
	positioned
	Items []ValueExpr
	Raw   string
}

func (ChoicesListClause) askItemNode() {}

// ChoicesReferenceClause preserves a dynamic choice source.
type ChoicesReferenceClause struct {
	positioned
	Source ValueExpr
}

func (ChoicesReferenceClause) askItemNode() {}

// RouteBranch preserves a single route branch.
type RouteBranch struct {
	positioned
	Predicate PredicateExpr
	Body      []ExecutionItem
	IsElse    bool
}

// PipelineStage preserves a pipeline stage and its nested body.
type PipelineStage struct {
	positioned
	Name Identifier
	Body []ExecutionItem
}
