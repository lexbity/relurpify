package thoughtrecipe

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// ValidateAgainstContracts validates a parsed document against the paradigm
// contract registry: every `uses X` binding must resolve to a registered
// contract, every directive carried by a run/delegate binding must be declared
// by the bound paradigm's contract with a matching grammar shape, nested
// items inside block directives must fall inside the declared Body set, and
// required directives must be present. It aggregates errors so one recipe load
// reports every violation, and it reads directive structures from the AST
// (names, forms, predicates, spans), which is the shape the P2 typed-directive
// change preserves.
func ValidateAgainstContracts(doc *ThoughtRecipeDocument, reg *paradigm.ContractRegistry) []error {
	if doc == nil {
		return nil
	}
	if reg == nil {
		reg = paradigm.Registry
	}

	agents := gatherAgentContractUses(doc)
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)

	var errs []error
	for _, name := range names {
		use := agents[name]
		contract, ok := reg.Lookup(use.paradigm)
		if !ok {
			errs = append(errs, &paradigm.ErrUnknownParadigm{
				Paradigm: use.paradigm,
				Valid:    reg.Names(),
				At:       locationFromSpan(use.span),
			})
			continue
		}
		for _, directive := range use.directives {
			errs = append(errs, validateDirectiveUse(directive, contract)...)
		}
		used := directiveNameSet(use.directives)
		for _, required := range contract.RequiredNames() {
			if !used[required] {
				errs = append(errs, &paradigm.ErrMissingRequiredDirective{
					Paradigm:  contract.Paradigm,
					Directive: required,
					At:        locationFromSpan(use.span),
				})
			}
		}
	}
	errs = append(errs, validateDocumentBlockRules(doc, reg, agents)...)
	return errs
}

// ValidatePlanContracts validates a compiled plan's paradigm-bound steps
// against the contract registry. Programmatic plans bypass the parser, so this
// is the registration-time gate (D3): it walks run/delegate steps in the plan,
// routes, pipelines, and pipeline stages, resolves each step's paradigm in the
// registry, and validates directive names from the typed `Directives` payloads.
// Structural steps (ask, capability, pipeline) carry the meta paradigm `euclo`
// and are not paradigm-bound, so they are skipped.
func ValidatePlanContracts(plan *ExecutionPlan, reg *paradigm.ContractRegistry) error {
	if plan == nil {
		return nil
	}
	if reg == nil {
		reg = paradigm.Registry
	}

	type paradigmUse struct {
		directives map[string]TypedDirective // first occurrence per directive name
	}
	uses := make(map[string]*paradigmUse)
	order := make([]string, 0)
	var errs []error

	var record func(step ExecutionStep)
	record = func(step ExecutionStep) {
		if step.Kind != StepKindRun && step.Kind != StepKindDelegate {
			for _, stage := range step.PipelineStages {
				walkPlanSteps(stage.Steps, record)
			}
			return
		}
		paradigmName := strings.TrimSpace(step.Paradigm)
		if paradigmName == "" {
			return
		}
		if contract, ok := reg.Lookup(paradigmName); ok {
			errs = append(errs, validateBlockRuleErrors(contract, step.Directives)...)
		}
		use, ok := uses[paradigmName]
		if !ok {
			use = &paradigmUse{directives: make(map[string]TypedDirective)}
			uses[paradigmName] = use
			order = append(order, paradigmName)
		}
		for _, directive := range step.Directives {
			if directive.Name == "" {
				continue
			}
			if _, seen := use.directives[directive.Name]; !seen {
				use.directives[directive.Name] = directive
			}
		}
		for _, stage := range step.PipelineStages {
			walkPlanSteps(stage.Steps, record)
		}
	}

	walkPlanSteps(plan.Steps, record)
	for _, route := range plan.Routes {
		for _, branch := range route.Branches {
			walkPlanSteps(branch.Steps, record)
		}
	}
	for _, pipeline := range plan.Pipelines {
		for _, stage := range pipeline.Stages {
			walkPlanSteps(stage.Steps, record)
		}
	}

	sort.Strings(order)
	for _, paradigmName := range order {
		use := uses[paradigmName]
		contract, ok := reg.Lookup(paradigmName)
		if !ok {
			errs = append(errs, &paradigm.ErrUnknownParadigm{
				Paradigm: paradigmName,
				Valid:    reg.Names(),
			})
			continue
		}
		used := make(map[string]bool, len(use.directives))
		for name, directive := range use.directives {
			used[name] = true
			spec, ok := contract.Directive(name)
			if !ok {
				errs = append(errs, &paradigm.ErrDirectiveNotInContract{
					Paradigm:  contract.Paradigm,
					Directive: name,
					Valid:     contract.DirectiveNames(),
				})
				continue
			}
			if spec.ArgsInteger {
				for _, arg := range directive.TextArgs {
					if !positiveIntArg(arg) {
						errs = append(errs, &paradigm.ErrDirectiveShape{
							Paradigm:  contract.Paradigm,
							Directive: name,
							Problem:   fmt.Sprintf("argument %q must be a positive integer", arg),
						})
					}
				}
			}
		}
		for _, required := range contract.RequiredNames() {
			if !used[required] {
				errs = append(errs, &paradigm.ErrMissingRequiredDirective{
					Paradigm:  contract.Paradigm,
					Directive: required,
				})
			}
		}
	}
	return errors.Join(errs...)
}

func walkPlanSteps(steps []ExecutionStep, record func(ExecutionStep)) {
	for _, step := range steps {
		record(step)
	}
}

// contractAgentUse aggregates the paradigm binding and every directive use
// attributed to one agent across all of its run/delegate blocks.
type contractAgentUse struct {
	paradigm   string
	span       SourceSpan
	directives []contractDirectiveUse
}

// contractDirectiveUse is one directive occurrence under an agent binding.
type contractDirectiveUse struct {
	name      string
	span      SourceSpan
	block     bool
	predicate bool
	textArgs  []string
	nested    []contractNestedUse
}

// contractNestedUse is one nested execution item inside a directive block body.
type contractNestedUse struct {
	bodyKind string
	name     string
	span     SourceSpan
}

func gatherAgentContractUses(doc *ThoughtRecipeDocument) map[string]*contractAgentUse {
	agents := make(map[string]*contractAgentUse)
	for _, decl := range doc.Declarations {
		agent, ok := decl.(*AgentDecl)
		if !ok {
			continue
		}
		name := strings.TrimSpace(agent.Name.Value)
		agents[name] = &contractAgentUse{
			paradigm: strings.TrimSpace(agent.AgentType.Value),
			span:     agent.GetSpan(),
		}
	}
	for _, decl := range doc.Declarations {
		switch node := decl.(type) {
		case *RunDecl:
			walkContractItems(node.Items, node.Agent.Value, agents)
		case *DelegateDecl:
			walkContractItems(node.Items, node.Agent.Value, agents)
		case *RouteDecl:
			for _, branch := range node.Branches {
				walkContractItems(branch.Body, "", agents)
			}
		case *PipelineDecl:
			for _, stage := range node.Stages {
				walkContractItems(stage.Body, "", agents)
			}
		}
	}
	return agents
}

func walkContractItems(items []ExecutionItem, agentName string, agents map[string]*contractAgentUse) {
	for _, item := range items {
		switch node := item.(type) {
		case *RunDecl:
			walkContractItems(node.Items, node.Agent.Value, agents)
		case *DelegateDecl:
			walkContractItems(node.Items, node.Agent.Value, agents)
		case *DirectiveClause:
			recordContractDirective(agentName, contractDirectiveUse{
				name:     strings.TrimSpace(node.Name.Value),
				span:     node.GetSpan(),
				textArgs: textArgsFromValueExprs(node.Arguments),
			}, agents)
		case *DirectiveBlock:
			var nested []contractNestedUse
			for _, child := range node.Body {
				nested = append(nested, classifyNestedUse(child))
			}
			recordContractDirective(agentName, contractDirectiveUse{
				name:      strings.TrimSpace(node.Name.Value),
				span:      node.GetSpan(),
				block:     true,
				predicate: node.Predicate != nil,
				textArgs:  textArgsFromValueExprs(node.Arguments),
				nested:    nested,
			}, agents)
			// Descend the block body WITHOUT recording nested directives as
			// paradigm-level uses: a clause nested inside a block is governed
			// by the block's declared Body set, not by the paradigm's
			// top-level directive vocabulary (e.g. chainer `link:` blocks
			// carry `prompt`/`from`/`capture` payloads that are not paradigm
			// directives). Nested run/delegate bindings still register their
			// own agent uses.
			walkContractBlockBody(node.Body, agentName, agents)
		case *RouteDecl:
			for _, branch := range node.Branches {
				walkContractItems(branch.Body, agentName, agents)
			}
		case *PipelineDecl:
			for _, stage := range node.Stages {
				walkContractItems(stage.Body, agentName, agents)
			}
		}
	}
}

// walkContractBlockBody descends a directive block's body looking only for
// nested agent bindings (run/delegate) and structural containers; it never
// records nested directives as top-level uses.
func walkContractBlockBody(items []ExecutionItem, agentName string, agents map[string]*contractAgentUse) {
	for _, item := range items {
		switch node := item.(type) {
		case *RunDecl:
			walkContractItems(node.Items, node.Agent.Value, agents)
		case *DelegateDecl:
			walkContractItems(node.Items, node.Agent.Value, agents)
		case *DirectiveBlock:
			walkContractBlockBody(node.Body, agentName, agents)
		case *RouteDecl:
			for _, branch := range node.Branches {
				walkContractBlockBody(branch.Body, agentName, agents)
			}
		case *PipelineDecl:
			for _, stage := range node.Stages {
				walkContractBlockBody(stage.Body, agentName, agents)
			}
		}
	}
}

func recordContractDirective(agentName string, use contractDirectiveUse, agents map[string]*contractAgentUse) {
	agent, ok := agents[agentName]
	if !ok || use.name == "" {
		return
	}
	agent.directives = append(agent.directives, use)
}

func classifyNestedUse(item ExecutionItem) contractNestedUse {
	switch node := item.(type) {
	case *RunDecl:
		return contractNestedUse{bodyKind: string(paradigm.BodyItemRun), span: node.GetSpan()}
	case *DelegateDecl:
		return contractNestedUse{bodyKind: string(paradigm.BodyItemDelegate), span: node.GetSpan()}
	case *CapabilityInvocation:
		return contractNestedUse{bodyKind: string(paradigm.BodyItemDo), span: node.GetSpan()}
	case *CaptureBlock:
		return contractNestedUse{bodyKind: string(paradigm.BodyItemCapture), span: node.GetSpan()}
	case *DirectiveClause:
		return contractNestedUse{bodyKind: string(paradigm.BodyItemDirective), name: strings.TrimSpace(node.Name.Value), span: node.GetSpan()}
	case *DirectiveBlock:
		return contractNestedUse{bodyKind: string(paradigm.BodyItemDirective), name: strings.TrimSpace(node.Name.Value), span: node.GetSpan()}
	case *GoalClause:
		return contractNestedUse{bodyKind: string(paradigm.BodyItemGoal), span: node.GetSpan()}
	case *FromClause:
		return contractNestedUse{bodyKind: string(paradigm.BodyItemFrom), span: node.GetSpan()}
	case *StreamClause:
		return contractNestedUse{bodyKind: string(paradigm.BodyItemStream), span: node.GetSpan()}
	case *ToolInvokePolicyDecl:
		return contractNestedUse{bodyKind: string(paradigm.BodyItemMay), span: node.GetSpan()}
	default:
		return contractNestedUse{span: item.GetSpan()}
	}
}

func validateDirectiveUse(use contractDirectiveUse, contract *paradigm.Contract) []error {
	spec, ok := contract.Directive(use.name)
	if !ok {
		return []error{&paradigm.ErrDirectiveNotInContract{
			Paradigm:  contract.Paradigm,
			Directive: use.name,
			Valid:     contract.DirectiveNames(),
			At:        locationFromSpan(use.span),
		}}
	}
	loc := locationFromSpan(use.span)
	if use.predicate && !spec.Predicate {
		return []error{&paradigm.ErrDirectiveShape{
			Paradigm:  contract.Paradigm,
			Directive: use.name,
			Problem:   "this directive declares no `when` predicate support",
			At:        loc,
		}}
	}
	if use.block && spec.Form != paradigm.FormBlock {
		return []error{&paradigm.ErrDirectiveShape{
			Paradigm:  contract.Paradigm,
			Directive: use.name,
			Problem:   fmt.Sprintf("block form is not declared; expected the line form"),
			At:        loc,
		}}
	}
	if !use.block && spec.Form == paradigm.FormBlock {
		return []error{&paradigm.ErrDirectiveShape{
			Paradigm:  contract.Paradigm,
			Directive: use.name,
			Problem:   "line form is not declared; expected a block directive",
			At:        loc,
		}}
	}
	if spec.ArgsInteger {
		for _, arg := range use.textArgs {
			if !positiveIntArg(arg) {
				return []error{&paradigm.ErrDirectiveShape{
					Paradigm:  contract.Paradigm,
					Directive: use.name,
					Problem:   fmt.Sprintf("argument %q must be a positive integer", arg),
					At:        loc,
				}}
			}
		}
	}
	if !use.block {
		return nil
	}
	var errs []error
	allowed := make(map[paradigm.BodyItem]bool, len(spec.Body))
	for _, body := range spec.Body {
		allowed[body] = true
	}
	for _, nested := range use.nested {
		if nested.bodyKind == "" {
			errs = append(errs, &paradigm.ErrDirectiveShape{
				Paradigm:  contract.Paradigm,
				Directive: use.name,
				Problem:   "contains an unsupported nested item",
				At:        locationFromSpan(nested.span),
			})
			continue
		}
		if !allowed[paradigm.BodyItem(nested.bodyKind)] {
			label := nested.bodyKind
			if nested.name != "" {
				label = fmt.Sprintf("%s %q", label, nested.name)
			}
			errs = append(errs, &paradigm.ErrDirectiveShape{
				Paradigm:  contract.Paradigm,
				Directive: use.name,
				Problem:   fmt.Sprintf("nested item %s is not allowed in a %s block", label, use.name),
				At:        locationFromSpan(nested.span),
			})
		}
	}
	return errs
}

func directiveNameSet(uses []contractDirectiveUse) map[string]bool {
	out := make(map[string]bool, len(uses))
	for _, use := range uses {
		out[use.name] = true
	}
	return out
}

// validateBlockRuleErrors applies the per-block directive rules declared by a
// contract: canonical order, Requires obligations (D1 mixing rule), and
// non-repeatable cardinality. It operates on the directives of one
// run/delegate block, so a rule violation is always attributable to an exact
// source position.
func validateBlockRuleErrors(contract *paradigm.Contract, directives []TypedDirective) []error {
	if contract == nil || len(directives) == 0 {
		return nil
	}
	var errs []error
	if err := validateOrder(contract.Paradigm, directives, contract.OrderRule()); err != nil {
		errs = append(errs, err)
	}
	errs = append(errs, validateDirectiveRequires(contract, directives)...)
	errs = append(errs, validateDirectiveCardinality(contract, directives)...)
	return errs
}

// validateDirectiveRequires enforces each directive's declared Requires set
// against the directives present in the same block (e.g. planner `step`
// requires `plan`).
func validateDirectiveRequires(contract *paradigm.Contract, directives []TypedDirective) []error {
	present := make(map[string]bool, len(directives))
	for _, directive := range directives {
		present[directive.Name] = true
	}
	var errs []error
	for _, directive := range directives {
		spec, ok := contract.Directive(directive.Name)
		if !ok || len(spec.Requires) == 0 {
			continue
		}
		for _, required := range spec.Requires {
			if present[required] {
				continue
			}
			errs = append(errs, &paradigm.ErrDirectiveRequires{
				Paradigm:  contract.Paradigm,
				Directive: directive.Name,
				Requires:  required,
				At:        locationFromSpan(directive.Span),
			})
		}
	}
	return errs
}

// validateDirectiveCardinality rejects a non-repeatable directive declared
// more than once in a block (e.g. two `verify` clauses).
func validateDirectiveCardinality(contract *paradigm.Contract, directives []TypedDirective) []error {
	seen := make(map[string]bool, len(directives))
	var errs []error
	for _, directive := range directives {
		spec, ok := contract.Directive(directive.Name)
		if !ok || spec.Repeatable {
			continue
		}
		if seen[directive.Name] {
			errs = append(errs, &paradigm.ErrDirectiveShape{
				Paradigm:  contract.Paradigm,
				Directive: directive.Name,
				Problem:   "declared more than once; this directive is not repeatable",
				At:        locationFromSpan(directive.Span),
			})
			continue
		}
		seen[directive.Name] = true
	}
	return errs
}

// positiveIntArg reports whether raw is a positive integer, tolerating the
// quoted form `"3"` that a StringLiteral argument renders as.
func positiveIntArg(raw string) bool {
	text := unquoteString(strings.TrimSpace(raw))
	if text == "" {
		return false
	}
	n, err := strconv.Atoi(text)
	return err == nil && n > 0
}

func locationFromSpan(span SourceSpan) paradigm.ContractLocation {
	return paradigm.ContractLocation{
		File:   span.Start.File,
		Line:   span.Start.Line,
		Column: span.Start.Column,
	}
}

// validateOrder checks the declaration order of a directive list against a
// paradigm's OrderRule. A directive whose canonical rank is lower than a
// predecessor's is a load error quoting both names and source positions; the
// first violation is returned. Names absent from the rule are unconstrained.
// The list is the directives of a single run/delegate block (order is a
// per-block property, never a per-paradigm aggregate).
func validateOrder(paradigmName string, directives []TypedDirective, rule *paradigm.OrderRule) error {
	if rule == nil || len(rule.Sequence) == 0 {
		return nil
	}
	rank := make(map[string]int, len(rule.Sequence))
	for i, name := range rule.Sequence {
		rank[name] = i
	}
	maxRank := -1
	var predecessor TypedDirective
	for _, directive := range directives {
		r, ok := rank[directive.Name]
		if !ok {
			continue
		}
		if r < maxRank {
			return &paradigm.ErrDirectiveOrder{
				Paradigm:  paradigmName,
				Directive: directive.Name,
				At:        locationFromSpan(directive.Span),
				After:     predecessor.Name,
				AfterAt:   locationFromSpan(predecessor.Span),
			}
		}
		maxRank = r
		predecessor = directive
	}
	return nil
}

// validateDocumentBlockRules walks every run/delegate block in the AST and
// applies the bound contract's per-block rules (order, Requires, cardinality).
// It is the load-time half; the plan-level half runs per compiled run/delegate
// step in ValidatePlanContracts and additionally covers route and pipeline
// bodies.
func validateDocumentBlockRules(doc *ThoughtRecipeDocument, reg *paradigm.ContractRegistry, agents map[string]*contractAgentUse) []error {
	if doc == nil {
		return nil
	}
	if reg == nil {
		reg = paradigm.Registry
	}
	var errs []error
	var check func(items []ExecutionItem, agentName string) []TypedDirective
	check = func(items []ExecutionItem, agentName string) []TypedDirective {
		var typed []TypedDirective
		for _, item := range items {
			switch item.(type) {
			case *DirectiveClause, *DirectiveBlock:
				typed = append(typed, lowerTypedDirective(item))
			}
		}
		if len(typed) == 0 || agentName == "" {
			return typed
		}
		agent, ok := agents[agentName]
		if !ok {
			return typed
		}
		contract, ok := reg.Lookup(agent.paradigm)
		if !ok {
			return typed
		}
		errs = append(errs, validateBlockRuleErrors(contract, typed)...)
		return typed
	}
	var walk func(items []ExecutionItem, agentName string)
	walk = func(items []ExecutionItem, agentName string) {
		check(items, agentName)
		for _, item := range items {
			switch node := item.(type) {
			case *RunDecl:
				walk(node.Items, node.Agent.Value)
			case *DelegateDecl:
				walk(node.Items, node.Agent.Value)
			case *RouteDecl:
				for _, branch := range node.Branches {
					walk(branch.Body, agentName)
				}
			case *PipelineDecl:
				for _, stage := range node.Stages {
					walk(stage.Body, agentName)
				}
			}
		}
	}
	for _, decl := range doc.Declarations {
		switch node := decl.(type) {
		case *RunDecl:
			walk(node.Items, node.Agent.Value)
		case *DelegateDecl:
			walk(node.Items, node.Agent.Value)
		case *RouteDecl:
			for _, branch := range node.Branches {
				walk(branch.Body, "")
			}
		case *PipelineDecl:
			for _, stage := range node.Stages {
				walk(stage.Body, "")
			}
		}
	}
	return errs
}
