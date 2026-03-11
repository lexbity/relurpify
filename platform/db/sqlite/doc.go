// Package sqlite provides agent-callable tools for executing SQL queries
// against SQLite database files within the agent's permitted workspace.
//
// Agents may run read-only SELECT queries or, where the manifest grants write
// permission, execute DML and DDL statements. Results are returned as
// structured tables the agent can reason over.
package sqlite
