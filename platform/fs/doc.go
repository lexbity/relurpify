// Package fs provides filesystem tools for reading, writing, and searching
// files within the agent's permitted workspace paths.
//
// files.go implements the core file tools (read_file, write_file, list_dir,
// find_files, file_exists) registered as agent capabilities. All operations
// are checked against the PermissionManager before execution.
//
// permission_cache.go caches the result of per-path permission lookups to
// avoid redundant policy evaluations on repeated accesses to the same files
// during a single agent iteration.
package fs
