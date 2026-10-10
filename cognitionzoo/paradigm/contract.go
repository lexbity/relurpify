package paradigm

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ExecutionShape classifies the execution structure a paradigm runs. The value
// is display/provenance vocabulary only; the shape a paradigm executes is a
// conformance-test obligation, not a runtime switch here.
type ExecutionShape string

const (
	ShapeIterative     ExecutionShape = "Iterative"     // react
	ShapeStagedPlan    ExecutionShape = "StagedPlan"    // rewoo
	ShapeDecomposition ExecutionShape = "Decomposition" // htn
	ShapeReviewLoop    ExecutionShape = "ReviewLoop"    // reflection
	ShapeChain         ExecutionShape = "Chain"         // chainer
	ShapeBlackboard    ExecutionShape = "Blackboard"    // blackboard
	ShapeLinear        ExecutionShape = "Linear"        // pipeline
	ShapePlanExecute   ExecutionShape = "PlanExecute"   // planner
)

// ValidExecutionShapes lists the closed set of declared execution shapes.
var ValidExecutionShapes = []ExecutionShape{ //nolint:gochecknoglobals // immutable contract vocabulary
	ShapeIterative,
	ShapeStagedPlan,
	ShapeDecomposition,
	ShapeReviewLoop,
	ShapeChain,
	ShapeBlackboard,
	ShapeLinear,
	ShapePlanExecute,
}

// DirectiveForm is the grammar form a directive takes: a single line
// (`until 3`) or an indented block (`step:` with a body).
type DirectiveForm string

const (
	FormLine  DirectiveForm = "line"
	FormBlock DirectiveForm = "block"
)

// ArgCard describes the quoted-string argument arity of a directive clause.
type ArgCard string

const (
	ArgNone ArgCard = "none"
	ArgOne  ArgCard = "one"
	ArgMany ArgCard = "many"
)

// BodyItem names a nested execution item a block-form directive may carry in
// its body. The set is the closed grammar-clause vocabulary; the spec's
// framework items are run|delegate|do|capture|directive, with from|goal|stream
// |may extending it to the clauses the parser actually recognizes inside a
// block body.
type BodyItem string

const (
	BodyItemRun       BodyItem = "run"
	BodyItemDelegate  BodyItem = "delegate"
	BodyItemDo        BodyItem = "do"
	BodyItemCapture   BodyItem = "capture"
	BodyItemDirective BodyItem = "directive"
	BodyItemFrom      BodyItem = "from"
	BodyItemGoal      BodyItem = "goal"
	BodyItemStream    BodyItem = "stream"
	BodyItemMay       BodyItem = "may"
)

// ValidBodyItems lists the closed set of nested body item names.
var ValidBodyItems = []BodyItem{ //nolint:gochecknoglobals // immutable contract vocabulary
	BodyItemRun,
	BodyItemDelegate,
	BodyItemDo,
	BodyItemCapture,
	BodyItemDirective,
	BodyItemFrom,
	BodyItemGoal,
	BodyItemStream,
	BodyItemMay,
}

// OrderRule declares the canonical declaration order of a paradigm's
// directives. A directive that appears before one of its declared predecessors
// in Sequence is a load error (see validateOrder). Directive names absent from
// Sequence are unconstrained: they carry no ordering obligation. Order is a
// per-run-block property, so the rule is checked against the directives of one
// run/delegate block, never against a per-paradigm aggregate.
type OrderRule struct {
	Sequence []string
}

// DirectiveSpec is the declared contract for one directive of a paradigm:
// its grammar form, argument arity, whether it may carry a `when` predicate,
// whether its single argument must be a positive integer (until-style caps),
// which nested items a block body may hold, whether it is repeatable, and
// whether the directive is required in every binding of the paradigm.
type DirectiveSpec struct {
	Name        string
	Form        DirectiveForm
	Text        ArgCard
	Predicate   bool
	ArgsInteger bool
	Body        []BodyItem
	Repeatable  bool
	Required    bool
}

// ConformanceCase declares one executable conformance obligation for a
// paradigm contract: a directive name and the runtime effect the paradigm must
// produce for it on the envelope. The conformance harness (`testsuite/`
// conformance, a later phase) executes these obligations; the contract's own
// unit test guarantees completeness — a declared directive without a case, or
// a case naming an undeclared directive, fails the contract's unit test.
type ConformanceCase struct {
	ID           string // stable case identifier, e.g. "react/until_bounds"
	Directive    string // directive name this case pins; "" for shape-only cases
	Assertion    string // prose: asserted runtime effect on the envelope
	FixtureShape string // prose: minimal recipe shape the case runs
}

// Contract is the declared contract for one cognitionzoo paradigm: the only
// valid `uses` spelling, the directives a recipe may target it with, the
// paradigms it composes internally, and its conformance obligations.
type Contract struct {
	Paradigm    string
	Summary     string
	Shape       ExecutionShape
	Directives  []DirectiveSpec
	Composes    []string
	Guarantees  []string
	Conformance []ConformanceCase
	// Order optionally declares the canonical declaration order of the
	// paradigm's directives. Nil means declaration order is unconstrained.
	Order *OrderRule
}

// OrderRule returns the contract's declared order rule, or nil when the
// paradigm imposes no ordering obligation.
func (c *Contract) OrderRule() *OrderRule {
	if c == nil {
		return nil
	}
	return c.Order
}

// Directive returns the declared spec for name, when present.
func (c *Contract) Directive(name string) (DirectiveSpec, bool) {
	if c == nil {
		return DirectiveSpec{}, false
	}
	for _, spec := range c.Directives {
		if spec.Name == name {
			return spec, true
		}
	}
	return DirectiveSpec{}, false
}

// DirectiveNames returns the declared directive names in declaration order.
func (c *Contract) DirectiveNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.Directives))
	for _, spec := range c.Directives {
		names = append(names, spec.Name)
	}
	return names
}

// RequiredNames returns the directives that MUST appear in every binding of
// the paradigm.
func (c *Contract) RequiredNames() []string {
	if c == nil {
		return nil
	}
	var required []string
	for _, spec := range c.Directives {
		if spec.Required {
			required = append(required, spec.Name)
		}
	}
	return required
}

// ContractLocation is the source position a contract error refers to. It
// mirrors the shape of the DSL's SourceLocation but lives in the vocabulary
// owner so contract errors can carry positions without importing the named
// layer (named imports cognitionzoo, never the reverse).
type ContractLocation struct {
	File   string
	Line   int
	Column int
}

// ContractRegistry is the collection of paradigm contracts. Paradigm packages
// register their Contract values here from init(); recipe load validates
// `uses X` and every directive against this registry.
type ContractRegistry struct {
	mu     sync.RWMutex
	byName map[string]*Contract
	order  []string
}

// NewContractRegistry creates an empty contract registry.
func NewContractRegistry() *ContractRegistry {
	return &ContractRegistry{
		byName: make(map[string]*Contract),
	}
}

// Register adds a contract. Duplicate or empty paradigm names are rejected,
// and the contract is checked for intrinsic consistency (directive names
// unique, forms and body items valid, and every declared directive pinned by a
// conformance case). Composes resolution is checked at test time by each
// paradigm's TestContractCompleteness, after all paradigm init()s have run.
func (r *ContractRegistry) Register(c Contract) error {
	if r == nil {
		return fmt.Errorf("contract registry is nil")
	}
	name := strings.TrimSpace(c.Paradigm)
	if name == "" {
		return fmt.Errorf("paradigm contract requires a name")
	}
	if name != c.Paradigm {
		return fmt.Errorf("paradigm contract name %q must not carry leading/trailing whitespace", c.Paradigm)
	}
	if errs := validateContractIntrinsic(c); len(errs) > 0 {
		return fmt.Errorf("invalid %s paradigm contract: %s", name, strings.Join(errStrings(errs), "; "))
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byName[name]; exists {
		return fmt.Errorf("duplicate paradigm contract registration for %q", name)
	}
	r.byName[name] = &c
	r.order = append(r.order, name)
	return nil
}

// Lookup returns the registered contract for the paradigm name.
func (r *ContractRegistry) Lookup(name string) (*Contract, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byName[strings.TrimSpace(name)]
	return c, ok
}

// Names returns all registered paradigm names in lexical order.
func (r *ContractRegistry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.order))
	for _, name := range r.order {
		if _, ok := r.byName[name]; ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// All returns all registered contracts in lexical order by paradigm name.
func (r *ContractRegistry) All() []Contract {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Contract, 0, len(r.order))
	for _, name := range r.order {
		if c, ok := r.byName[name]; ok {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Paradigm < out[j].Paradigm })
	return out
}

// Registry is the process-wide contract collection. Every cognitionzoo
// paradigm registers its Contract from init(); the DSL validator resolves
// `uses X` against this registry at load time.
var Registry = NewContractRegistry() //nolint:gochecknoglobals // process-wide contract collection, populated by paradigm init()s

// Register adds a contract to the process-wide registry from a paradigm
// package's init(). A duplicate or invalid registration is a programming
// error; callers that need a scoped registry use NewContractRegistry.
func Register(c Contract) error {
	return Registry.Register(c)
}

func errStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			out = append(out, err.Error())
		}
	}
	return out
}

func validateContractIntrinsic(c Contract) []error {
	var errs []error
	if !validShape(c.Shape) {
		errs = append(errs, fmt.Errorf("unknown execution shape %q", c.Shape))
	}
	seen := make(map[string]struct{}, len(c.Directives))
	for _, spec := range c.Directives {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			errs = append(errs, fmt.Errorf("directive with empty name"))
			continue
		}
		if _, dup := seen[name]; dup {
			errs = append(errs, fmt.Errorf("duplicate directive %q", name))
		}
		seen[name] = struct{}{}
		if spec.Form != FormLine && spec.Form != FormBlock {
			errs = append(errs, fmt.Errorf("directive %q has invalid form %q", name, spec.Form))
		}
		switch spec.Text {
		case ArgNone, ArgOne, ArgMany:
		default:
			errs = append(errs, fmt.Errorf("directive %q has invalid arg card %q", name, spec.Text))
		}
		for _, body := range spec.Body {
			if !validBodyItem(body) {
				errs = append(errs, fmt.Errorf("directive %q allows invalid body item %q", name, body))
			}
		}
	}
	// RequiredNames derives from DirectiveSpec.Required, so a required name is
	// declared by construction; no separate cross-check exists here.
	if c.Order != nil {
		for _, name := range c.Order.Sequence {
			if _, ok := seen[name]; !ok {
				errs = append(errs, fmt.Errorf("order rule references undeclared directive %q", name))
			}
		}
	}
	for _, composed := range c.Composes {
		if strings.TrimSpace(composed) == "" {
			errs = append(errs, fmt.Errorf("composes entry must be a paradigm name"))
		}
	}
	errs = append(errs, validateConformanceCases(c)...)
	return errs
}

func validateConformanceCases(c Contract) []error {
	var errs []error
	declared := make(map[string]struct{}, len(c.Directives))
	for _, spec := range c.Directives {
		declared[spec.Name] = struct{}{}
	}
	pinned := make(map[string]struct{}, len(c.Directives))
	ids := make(map[string]struct{}, len(c.Conformance))
	for _, cas := range c.Conformance {
		if strings.TrimSpace(cas.ID) == "" {
			errs = append(errs, fmt.Errorf("conformance case with empty ID"))
			continue
		}
		if _, dup := ids[cas.ID]; dup {
			errs = append(errs, fmt.Errorf("duplicate conformance case ID %q", cas.ID))
		}
		ids[cas.ID] = struct{}{}
		directive := strings.TrimSpace(cas.Directive)
		if directive == "" {
			continue // shape-level case; no directive pin
		}
		if _, ok := declared[directive]; !ok {
			errs = append(errs, fmt.Errorf("conformance case %q pins undeclared directive %q", cas.ID, directive))
			continue
		}
		pinned[directive] = struct{}{}
	}
	for _, spec := range c.Directives {
		if _, ok := pinned[spec.Name]; !ok {
			errs = append(errs, fmt.Errorf("directive %q has no conformance case", spec.Name))
		}
	}
	return errs
}

func validShape(shape ExecutionShape) bool {
	for _, known := range ValidExecutionShapes {
		if shape == known {
			return true
		}
	}
	return false
}

func validBodyItem(item BodyItem) bool {
	for _, known := range ValidBodyItems {
		if item == known {
			return true
		}
	}
	return false
}

// ErrUnknownParadigm reports a paradigm name that is not registered in the
// contract registry. Recipe load rejects it with the location and the valid
// paradigm names; the message keeps the historical "unsupported agent
// paradigm" wording so existing diagnostics stay greppable.
type ErrUnknownParadigm struct {
	Paradigm string
	Valid    []string
	At       ContractLocation
}

func (e *ErrUnknownParadigm) Error() string {
	return locationPrefix(e.At) + fmt.Sprintf("unsupported agent paradigm %q; valid paradigms: %s",
		e.Paradigm, strings.Join(e.Valid, ", "))
}

// ErrDirectiveNotInContract reports a directive the bound paradigm's contract
// does not declare.
type ErrDirectiveNotInContract struct {
	Paradigm  string
	Directive string
	Valid     []string
	At        ContractLocation
}

func (e *ErrDirectiveNotInContract) Error() string {
	return locationPrefix(e.At) + fmt.Sprintf("directive %q is not declared by the %s paradigm contract; valid directives: %s",
		e.Directive, e.Paradigm, strings.Join(e.Valid, ", "))
}

// ErrMissingRequiredDirective reports a required directive with no occurrence
// in any binding of the paradigm.
type ErrMissingRequiredDirective struct {
	Paradigm  string
	Directive string
	At        ContractLocation
}

func (e *ErrMissingRequiredDirective) Error() string {
	return locationPrefix(e.At) + fmt.Sprintf("required directive %q is missing for the %s paradigm",
		e.Directive, e.Paradigm)
}

// ErrDirectiveShape reports a grammar-shape violation against a declared
// directive: block form where line is declared (or the reverse), a predicate
// on a directive that declares none, or a nested body item outside the
// declared Body set.
type ErrDirectiveShape struct {
	Paradigm  string
	Directive string
	Problem   string
	At        ContractLocation
}

func (e *ErrDirectiveShape) Error() string {
	return locationPrefix(e.At) + fmt.Sprintf("directive %q violates the %s paradigm contract: %s",
		e.Directive, e.Paradigm, e.Problem)
}

// ErrDirectiveOrder reports a directive declared out of the paradigm
// contract's canonical order (e.g. `synthesize` before `plan`). It quotes both
// the offending directive and the predecessor it must appear before, with
// their source positions.
type ErrDirectiveOrder struct {
	Paradigm  string
	Directive string
	At        ContractLocation
	After     string
	AfterAt   ContractLocation
}

func (e *ErrDirectiveOrder) Error() string {
	afterLabel := "declared earlier"
	if strings.TrimSpace(e.AfterAt.File) != "" {
		afterLabel = fmt.Sprintf("declared at %s:%d:%d", e.AfterAt.File, e.AfterAt.Line, e.AfterAt.Column)
	}
	return locationPrefix(e.At) + fmt.Sprintf("directive %q in the %s paradigm contract is out of order: it must appear before %q (%s)",
		e.Directive, e.Paradigm, e.After, afterLabel)
}

// ErrContractViolation is the runtime guard for a paradigm outside the
// contract registry, or a step whose directive payloads fail option lowering.
// It is reachable only when a caller constructs an ExecutionPlan bypassing the
// loader and registration validation; an .erpe-reachable authoring path rejects
// the same mistake at load time.
type ErrContractViolation struct {
	Step     string
	Paradigm string
	// Cause is the underlying load-time lowering error (e.g. a missing `do`
	// clause), when present. It preserves the specific diagnostic while keeping
	// the typed guard for errors.As.
	Cause error
}

func (e *ErrContractViolation) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("thoughtrecipe step %q violates the paradigm contract: %v", e.Step, e.Cause)
	}
	return fmt.Sprintf("thoughtrecipe step %q violates the paradigm contract: paradigm %q has no registered contract",
		e.Step, e.Paradigm)
}

// Unwrap exposes the underlying lowering error for errors.Is/As.
func (e *ErrContractViolation) Unwrap() error { return e.Cause }

func locationPrefix(loc ContractLocation) string {
	if strings.TrimSpace(loc.File) == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d: ", loc.File, loc.Line, loc.Column)
}
