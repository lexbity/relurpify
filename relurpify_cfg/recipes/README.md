# Thought Recipe Examples

This directory contains example thought recipe YAML files demonstrating the Euclo Thought Recipes feature.

The examples use the spec-shaped schema:
- `apiVersion: relurpify/v1alpha1`
- `metadata.name`
- `global.configuration`
- `global.context`
- `sequence[*].mutation`
- `sequence[*].hitl`
- `sequence[*].parent.paradigm`
- `sequence[*].parent.prompt`

## Files

### `simple-react.yaml`
Basic single-step recipe using the React paradigm with file read/write capabilities.

**Features:**
- Single execution step
- Capability filtering (file_read, file_write)
- Carry-forward sharing mode

**Use case:** Simple, quick tasks requiring basic file operations.

---

### `htn-with-child.yaml`
HTN (Hierarchical Task Network) planning with a React child agent for primitive execution.

**Features:**
- Delegation paradigm (HTN with child)
- Different capabilities for parent and child
- Explicit sharing mode

**Use case:** Complex planning tasks requiring hierarchical decomposition.

---

### `multi-step-capture.yaml`
Three-step recipe demonstrating data capture and context sharing across steps.

**Features:**
- Multiple steps with dependencies
- Data capture (`capture` field)
- Context variable usage (`{{context.explored_files}}`)
- Enrichment sources (AST, archaeology)
- Child agent in reflection step

**Use case:** Multi-phase workflows (explore → analyze → implement).

---

### `with-fallback.yaml`
Recipe with fallback agent for handling child failures.

**Features:**
- Fallback agent configuration
- Isolated sharing mode
- Goalcon paradigm with React child and Planner fallback

**Use case:** Robust execution requiring backup strategies.

---

### `global-restricted.yaml`
Globally restricted capabilities for read-only analysis tasks.

**Features:**
- Global capability restrictions (read-only)
- Global enrichment (AST, archaeology)
- Single analysis step

**Use case:** Safe, read-only codebase analysis.

## Usage

To use these recipes, place them in your recipe directory and reference them by capability ID:

```yaml
# Capability ID format: euclo:recipe.<recipe-id>
# Example: euclo:recipe.simple-react
```

The recipe will be triggered when:
1. The trigger keywords match the task instruction
2. The recipe is loaded into the `PlanRegistry`
3. The dispatcher routes the capability ID

## Creating Custom Recipes

1. Copy an example as a template
2. Modify `metadata.id` to be unique
3. Adjust `global.configuration.intent_keywords` for your use case
4. Configure `sequence` with your workflow
5. Save to the recipes directory

See `docs/plans/euclo-reimplementation-spec.md` for full documentation.
