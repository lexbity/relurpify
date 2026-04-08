# Codesmells: `app/nexus/db`

Captured while adding phase-4 coverage around the Nexus SQLite persistence layer.

## Observations

- Several stores embed schema bootstrap and migration behavior directly in `init` methods. That is pragmatic for a small SQLite-backed repo, but the repeated `CREATE TABLE` plus `ALTER TABLE` patterns make long-term migrations easy to miss.
- `sqlite_session_store.go` in particular has a dense initialization path with duplicate-column tolerance, backfill updates, and multiple legacy column branches. It is operationally sensible, but it is the kind of code that benefits from explicit regression tests because the intent is easy to lose in the branching.
- The audit-chain store combines canonical hashing, signature handling, and verification logic with SQL persistence in a single path. The logic is coherent, but the reader has to mentally separate data integrity rules from storage mechanics.
- The compatibility-window store is much simpler, which is good, but it is also a reminder that the package mixes both very small and very large persistence adapters. That unevenness increases review cost because each file has different failure modes.

## Tradeoff Note

- The SQLite-backed persistence layer is intentionally direct and schema-local. The risk is not architectural mismatch, but maintenance burden around migration semantics and regression coverage when the schema evolves.
