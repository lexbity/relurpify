# Nexus Identity, Tenancy, and Session Security Implementation Plan

This document is a working copy of the current implementation plan for Nexus identity, tenancy, and session security. It is based on the active review of the Relurpify framework and Nexus codebase and is organized into eight execution phases that match the current repository layout.

## Goal

Complete the Nexus security model so that:

- every gateway action is tied to an authenticated principal
- every session, node, and external identity is tenant-scoped
- tenant boundaries are enforced consistently across gateway, session, admin, and event surfaces

## Phase 1: Identity Foundation

Target:

- finish the missing durable identity model
- add tenant and subject persistence alongside the existing external identity and node enrollment records
- make issued bearer tokens tenant-aware at the metadata level

Deliverables:

- additive tenant and subject records in `framework/core`
- expanded `framework/identity.Store`
- SQLite support for tenants and subjects
- tenant-aware token metadata fields in the admin token store
- validation and round-trip tests

Notes:

- this phase is additive only
- runtime enforcement changes should remain compatible with existing callers

## Phase 2: Principal and Token Model

Target:

- resolve runtime-issued tokens to tenant-scoped principals rather than default-tenant placeholders

Deliverables:

- subject-bound token issuance
- tenant-aware principal resolution
- tests for tenant and scope correctness

## Phase 3: Tenant-Aware Admin Authorization

Target:

- require admin resources and tools to respect tenant scope

Deliverables:

- principal-to-tenant authz helpers
- tenant-checked session, node, token, and event access
- cross-tenant denial tests

## Phase 4: Node Enrollment as Source of Truth

Target:

- make stored enrollment authoritative for node identity, trust, and approved metadata

Deliverables:

- enrollment-backed node connection flow
- removal of insecure default descriptor fallback in secure pairing paths
- spoofed node metadata rejection tests

## Phase 5: External Identity Resolution and Binding Policy

Target:

- distinguish bound, unbound, and restricted external identities before session routing

Deliverables:

- richer external identity resolution result types
- provider-binding data on policy requests
- tests for restricted handling of unresolved identities

## Phase 6: Session Authorization Hardening

Target:

- enforce session authorization using tenant, owner, delegation, and provider binding

Deliverables:

- session authz helpers
- explicit delegation records
- impersonation and cross-tenant denial tests

## Phase 7: Event Stream Isolation

Target:

- ensure admin and runtime event reads are tenant-safe and projection-based where needed

Deliverables:

- tenant-safe event access model
- scoped runtime/admin event feeds
- audit coverage for subscription decisions

## Phase 8: Bootstrap and Operator UX

Target:

- make the secure model operable through bootstrap, admin, and Nexusish workflows

Deliverables:

- tenant and subject bootstrap flows
- tenant-aware node approval UX
- visibility into tenants, subjects, enrollments, and bindings

## Current Status

Phase 1 is in progress in the repository. The first code changes focus on:

- tenant and subject record types
- tenant and subject SQLite persistence
- tenant-aware token metadata
- compatibility-safe principal resolution fallback
