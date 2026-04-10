// Package fs provides filesystem tools for reading, writing, and searching
// files within the agent's permitted workspace paths.
//
// files.go implements the core file tools (read_file, write_file, list_dir,
// find_files, file_exists) registered as agent capabilities. The sandbox file
// scope is enforced before host I/O, and the PermissionManager remains the
// policy backstop for manifest-driven approvals and matrices.
//
// permission_cache.go caches the result of per-path permission lookups to
// avoid redundant policy evaluations on repeated accesses to the same files
// during a single agent iteration.
package fs
