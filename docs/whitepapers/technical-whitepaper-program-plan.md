# Technical Whitepaper Program Plan

## Purpose

This document defines a multi-phase writing plan for a coherent set of
technical whitepapers covering three major Relurpify boundaries:

- `agents/`
- `named/euclo/`
- `archaeo/`

The goal is not to produce marketing copy or duplicate package READMEs. The
goal is to produce durable technical papers that explain why these boundaries
exist, what they own, how they compose, and what implementation evidence in the
repository supports their claims.

This plan assumes the papers may later be adapted for external publication, but
the initial versions should be written as internal-source-of-truth technical
documents grounded in the current codebase.

## Proposed Paper Set

The program should be split into three primary whitepapers plus one optional
framing note.

### Primary whitepapers

- `agents` whitepaper
  - generic execution paradigms
  - runtime layering and dependency rules
  - why generic paradigms are separate from named runtimes

- `euclo` whitepaper
  - named coding runtime
  - mode system and `UnitOfWork`
  - relurpic capability ownership
  - composition of generic paradigms under Euclo orchestration

- `archaeo` whitepaper
  - archaeology intelligence
  - durable provenance, learning, tensions, projections, and living plans
  - execution handoff boundaries with Euclo and UX surfaces

### Optional framing note

- Relurpify runtime overview
  - `framework -> agents -> named/euclo -> app`
  - where `archaeo` sits as a root domain runtime
  - why these papers are separate instead of collapsed into a single document

## Current State

The repo already contains substantial architecture material, but it is spread
across package docs, README files, and engineering specifications:

- `docs/agents/README.md`
- `docs/agents/euclo.md`
- `docs/framework/layering.md`
- `named/euclo/README.md`
- `archaeo/README.md`
- `docs/archaeo/archeo-prototype-spec.md`

This means the writing problem is not lack of source material. The problem is:

- narrative duplication
- inconsistent depth across boundaries
- unclear separation between reference docs and whitepaper-style argumentation
- some terminology drift between older and newer documents

The whitepaper plan should therefore focus on synthesis, boundary clarity, and
terminology normalization rather than speculative invention.

## Program Goals

1. Explain each boundary in terms of ownership, runtime semantics, and artifact
   model rather than package inventory alone.
2. Keep dependency direction and layering claims consistent with the current
   codebase.
3. Use implementation evidence from the repository to support claims whenever
   possible.
4. Avoid collapsing `agents`, `euclo`, and `archaeo` into one narrative, since
   that would hide the architecture the repo is explicitly building toward.
5. Surface current limitations and transitional seams honestly instead of
   presenting unfinished areas as already fully mature.

## Non-Goals

- Rewriting all existing docs before drafting the papers.
- Publishing polished external PDFs in the first pass.
- Converting every package doc into academic-style prose.
- Treating the whitepapers as immutable specifications.

## Source Base By Paper

### `agents` whitepaper

Primary inputs:

- `docs/agents/README.md`
- `agents/doc.go`
- paradigm docs under `docs/agents/`
- package docs under `agents/*/doc.go`

Key questions to answer:

- why generic paradigms are their own layer
- how paradigms differ operationally
- how named agents compose paradigms instead of inheriting their identity from
  one of them
- what runtime semantics matter in practice: planning, persistence,
  checkpoints, specialist dispatch, verification loops, and resumability

### `euclo` whitepaper

Primary inputs:

- `docs/agents/euclo.md`
- `named/euclo/README.md`
- `named/euclo/doc.go`
- Euclo runtime and execution package structure

Key questions to answer:

- what makes Euclo a named runtime rather than a thin wrapper around one agent
  paradigm
- how `UnitOfWork` organizes execution
- how Euclo modes differ from UX labels
- how Euclo owns relurpic behavior while composing `/agents` paradigms
- how Euclo interacts with `framework` policy and `archaeo` state

### `archaeo` whitepaper

Primary inputs:

- `archaeo/README.md`
- `docs/archaeo/archeo-prototype-spec.md`
- `archaeo/domain/*`
- `archaeo/phases/*`
- `archaeo/plans/*`
- `archaeo/projections/*`
- `archaeo/bindings/euclo/*`

Key questions to answer:

- what archaeology intelligence is and why it differs from execution
  intelligence
- why archaeology findings should become durable artifacts
- how learning, tensions, convergence, and living plans fit together
- why `archaeo` is a root namespace instead of a Euclo subpackage
- how execution handoff and verification return information back into the
  archaeology domain

## Recommended Writing Order

The program should proceed in this order:

1. `agents`
2. `euclo`
3. `archaeo`
4. optional runtime overview note

This order reduces rework:

- `agents` establishes the generic execution vocabulary.
- `euclo` can then reference those paradigms without re-explaining them.
- `archaeo` can then explain durable reasoning and living-plan state with the
  Euclo execution boundary already defined.

## Phase Plan

## Phase 1: Audience, Scope, and Publication Boundary

### Objective

Define what each paper is trying to do and what it is not trying to do.

### Work

- choose the primary audience for the initial drafts:
  - internal engineering alignment
  - technically sophisticated external readers
  - future partner or investor diligence
- choose the disclosure boundary:
  - internal-only implementation detail allowed
  - architecture-public but code-sensitive details omitted
- define the standard tone:
  - engineering whitepaper
  - architecture rationale
  - implementation-backed technical narrative

### Deliverables

- one paragraph audience statement for the full paper set
- one paragraph audience statement per paper
- approved disclosure boundary

### Exit Criteria

- each paper has a clear reader and objective
- the program knows what to omit as well as what to include

## Phase 2: Source Mapping and Claim Inventory

### Objective

Build a repository-grounded source map so the papers rely on real code and docs
instead of memory or ad hoc interpretation.

### Work

- enumerate canonical input docs for each paper
- map code packages that provide direct evidence for each architectural claim
- capture terminology collisions:
  - agent vs runtime
  - mode vs UX surface
  - archaeology vs planning
  - capability vs relurpic capability
  - execution vs reasoning
- identify claims that are aspirational rather than fully implemented

### Deliverables

- source index per whitepaper
- claim ledger with:
  - claim
  - supporting files
  - maturity status: implemented / partial / aspirational

### Exit Criteria

- every major section in each paper has known source support
- known speculative areas are explicitly marked before drafting begins

## Phase 3: Whitepaper Architecture and Shared Template

### Objective

Create a repeatable structure so the papers read as a program rather than three
unrelated essays.

### Proposed shared sections

- Abstract
- Problem
- Why common agent framings are insufficient
- Ownership boundary
- Runtime model
- Artifact and data model
- Lifecycle and control flow
- Operational properties
- Current implementation status
- Constraints and open problems

### Work

- define which sections are mandatory across all papers
- define which sections are paper-specific:
  - paradigm comparison tables for `agents`
  - mode and `UnitOfWork` detail for `euclo`
  - events, projections, and living-plan lifecycle for `archaeo`
- define diagram list for each paper

### Deliverables

- shared whitepaper template
- per-paper section outlines
- diagram backlog

### Exit Criteria

- all papers have approved outlines
- the program has a consistent rhetorical structure

## Phase 4: Draft the `agents` Whitepaper

### Objective

Explain the generic execution layer clearly enough that later papers can build
on it without restating the whole architecture.

### Focus

- generic paradigm ownership under `agents/`
- separation from `framework/` and `named/`
- practical differences between runtime patterns:
  - ReAct
  - Planner
  - HTN
  - ReWOO
  - Blackboard
  - Chainer
  - Pipeline
  - Reflection
  - Architect
- tradeoffs between control styles, persistence, and determinism

### Draft outputs

- section outline
- first draft
- paradigm comparison table
- dependency-boundary callout

### Exit Criteria

- a reader can explain why `agents/` exists as its own layer
- a reader can distinguish paradigm identity from named runtime identity

## Phase 5: Draft the `euclo` Whitepaper

### Objective

Present Euclo as a named coding runtime with its own orchestration model,
rather than as a branding wrapper around a single generic agent pattern.

### Focus

- Euclo as a named runtime
- `UnitOfWork` as the core execution contract
- chat, planning, and debug as execution modes
- relurpic capability ownership
- Euclo recipe execution over `/agents`
- relationship with `framework` policy, `archaeo` memory, and `relurpish`

### Draft outputs

- section outline
- first draft
- mode-to-owner-to-executor mapping diagram
- continuity / restore / deferral explanation

### Exit Criteria

- the paper clearly shows why Euclo is not reducible to ReAct, Planner, HTN,
  or any other one paradigm
- the paper makes the Euclo ownership boundary legible to someone reading the
  repo for the first time

## Phase 6: Draft the `archaeo` Whitepaper

### Objective

Explain Archaeo as a durable archaeology and living-plan runtime rather than a
sidecar feature set attached to an execution agent.

### Focus

- archaeology intelligence vs execution intelligence
- exploration sessions, snapshots, learning interactions, tensions,
  convergence, and living plans
- events and projections as durable domain surfaces
- bindings to Euclo and Relurpish
- why archaeology reasoning must survive outside transient LLM context

### Draft outputs

- section outline
- first draft
- lifecycle diagram from archaeology through execution handoff and return
- artifact model table

### Exit Criteria

- the paper clearly explains why `archaeo` is a root namespace
- the paper makes the durable-domain argument using concrete repository
  evidence rather than abstract LLM rhetoric alone

## Phase 7: Cross-Paper Consistency Pass

### Objective

Remove contradictions and drift across the paper set.

### Work

- normalize terminology
- align boundary descriptions
- remove repeated explanations where one paper should instead refer to another
- check all architectural claims against current implementation
- ensure partial or future-state claims are labeled honestly

### Deliverables

- terminology sheet
- revised drafts with aligned language
- unresolved terminology or architecture questions list

### Exit Criteria

- the three papers read as one coherent architecture program
- no major ownership contradiction remains between them

## Phase 8: Publication Packaging

### Objective

Prepare the papers for durable use inside the repo and optional external
adaptation.

### Suggested locations

- `docs/whitepapers/agents-runtime.md`
- `docs/whitepapers/euclo-runtime.md`
- `docs/whitepapers/archaeo-runtime.md`
- optional `docs/whitepapers/relurpify-runtime-overview.md`

### Work

- add front matter or consistent title structure
- add diagrams
- add cross-links to existing reference docs
- add a short note describing the relationship between whitepapers and package
  documentation

### Exit Criteria

- the paper set is readable in-repo without requiring IDE context
- readers can move from whitepapers to reference docs and code without
  confusion

## Risks and Failure Modes

- papers drift into marketing language and stop being technically useful
- papers duplicate README material without adding architecture rationale
- `archaeo` claims overstate what is already implemented versus what is still
  transitional
- terminology differences between older docs and newer package structure create
  contradictions
- one oversized “master paper” absorbs all topics and destroys boundary clarity

## Recommended Immediate Next Steps

1. Create the three-paper outline set before drafting prose.
2. Build a terminology ledger and claim ledger in parallel.
3. Draft `agents` first and use it to lock vocabulary for `euclo`.
4. Draft `euclo` second and use it to lock the execution boundary for
   `archaeo`.
5. Draft `archaeo` last because it has the heaviest domain and lifecycle
   surface.

## Minimal Deliverable Schedule

If the program needs a lightweight execution path, use this sequence:

1. day 1: audience statement, source map, terminology ledger
2. day 2: `agents` outline and first draft
3. day 3: `euclo` outline and first draft
4. day 4: `archaeo` outline and first draft
5. day 5: cross-paper consistency pass and publication packaging

That schedule is only appropriate if the purpose is internal alignment. Any
external-facing publication should assume additional review for confidentiality,
precision, and diagram quality.
