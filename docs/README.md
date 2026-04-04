# Documentation

This directory is organized by codebase boundary and audience.

## Getting Started

| Document | Description |
|----------|-------------|
| getting-started/installation.md | Prerequisites and first install |
| getting-started/workspace-layout.md | Canonical `relurpify_cfg/` layout and ownership rules |

## Framework

| Document | Description |
|----------|-------------|
| framework/architecture.md | System overview and layering |
| framework/framework.md | Framework package map and runtime details |
| framework/configuration.md | Workspace config, manifests, and policy surfaces |
| framework/permission-model.md | Enforcement and HITL model |
| framework/retrieval.md | Retrieval and embedding flow |
| framework/middleware.md | MCP and Nexus transport layers |
| framework/mcp.md | MCP capability model and lifecycle |
| framework/layering.md | Dependency rules and four-layer architecture |

## Agents

| Document | Description |
|----------|-------------|
| agents/README.md | Named agents and generic paradigm overview |
| agents/euclo.md | Euclo — primary coding agent |
| agents/architect.md | Architect paradigm |
| agents/react.md | ReAct paradigm |
| agents/pipeline.md | Pipeline paradigm |
| agents/blackboard.md | Blackboard paradigm |
| agents/custom-agents.md | How to build custom agents |

## Composition Root

| Document | Description |
|----------|-------------|
| ayenitd/ayenitd.md | ayenitd — service lifecycle manager, Open() entry point, WorkspaceEnvironment |

## Applications

| Document | Description |
|----------|-------------|
| apps/README.md | Overview of all application binaries |
| apps/relurpish.md | relurpish TUI guide |
| apps/dev-agent.md | dev-agent CLI guide |
| nexus/nexus.md | Nexus gateway overview |
| apps/nexusish.md | nexusish admin TUI overview |

## Nexus

| Document | Description |
|----------|-------------|
| nexus/nexus.md | Nexus gateway overview |
| nexus/nexus-admin-api.md | Admin API architecture |
| nexus/federated_mesh_protocol_engineering_spec.md | Federated Mesh Protocol specification |
