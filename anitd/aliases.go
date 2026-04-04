package anitd

// This file contains backward-compatibility type aliases.
// They will be removed after migration is complete.

// WorkspaceEnvironmentAlias is an alias for WorkspaceEnvironment to help
// with gradual migration. Use WorkspaceEnvironment directly in new code.
type WorkspaceEnvironmentAlias = WorkspaceEnvironment

// WorkspaceConfigAlias is an alias for WorkspaceConfig.
type WorkspaceConfigAlias = WorkspaceConfig
