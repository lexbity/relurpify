package authorization

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// LiftedPermissions aggregates all virtual operations statically extracted
// from a shell command string.
type LiftedPermissions struct {
	FileSystem  []permissions.FileSystemPermission
	Executables []permissions.ExecutablePermission
	Network     []permissions.NetworkPermission
	HasDynamic  bool
}

// LiftShellCommand parses an arbitrary POSIX/bash command string and walks its
// AST to statically lift low-level commands to high-level virtual permissions.
func LiftShellCommand(cmdStr string) (*LiftedPermissions, error) {
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return &LiftedPermissions{}, nil
	}

	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(cmdStr), "")
	if err != nil {
		return nil, err
	}

	res := &LiftedPermissions{}

	// Walk the AST nodes deterministically
	syntax.Walk(file, func(node syntax.Node) bool {
		if node == nil {
			return true
		}

		switch n := node.(type) {
		case *syntax.CmdSubst:
			// Detects `$()` or backtick command substitution
			res.HasDynamic = true

		case *syntax.CallExpr:
			if len(n.Args) == 0 {
				return true
			}

			// If the command name is not a simple literal, it is dynamic execution
			binary := wordToLiteral(n.Args[0])
			if binary == "" {
				res.HasDynamic = true
				return true
			}

			binary = strings.ToLower(binary)

			// Map binary command to virtual operations
			switch binary {
			case "eval":
				res.HasDynamic = true

			case "cat", "head", "tail", "less", "more", "bat":
				// FileSystemRead operations
				paths := extractArgPaths(n.Args[1:])
				for _, p := range paths {
					res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{
						Action: permissions.FileSystemRead,
						Path:   p,
					})
				}

			case "rm", "rmdir", "shred", "unlink":
				// FileSystemDelete operations
				paths := extractArgPaths(n.Args[1:])
				for _, p := range paths {
					res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{
						Action: permissions.FileSystemDelete,
						Path:   p,
					})
				}

			case "cp", "mv":
				// Source files are read, destination is written
				paths := extractArgPaths(n.Args[1:])
				if len(paths) > 0 {
					// Last path is destination, others are sources
					dest := paths[len(paths)-1]
					sources := paths[:len(paths)-1]
					for _, src := range sources {
						res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{
							Action: permissions.FileSystemRead,
							Path:   src,
						})
					}
					res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{
						Action: permissions.FileSystemWrite,
						Path:   dest,
					})
				}

			case "sed":
				// In-place edit is FileSystemWrite, others are FileSystemRead
				isInPlace := false
				var paths []string
				for _, arg := range n.Args[1:] {
					val := wordToLiteral(arg)
					if val == "" {
						continue
					}
					if val == "-i" || val == "--in-place" || strings.HasPrefix(val, "-i") {
						isInPlace = true
					} else if !strings.HasPrefix(val, "-") {
						paths = append(paths, val)
					}
				}
				// The last path (usually target file) is written if in-place, others are read
				if len(paths) > 0 {
					target := paths[len(paths)-1]
					action := permissions.FileSystemRead
					if isInPlace {
						action = permissions.FileSystemWrite
					}
					res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{
						Action: action,
						Path:   target,
					})
				}

			case "tee":
				// Writes to the specified target files
				paths := extractArgPaths(n.Args[1:])
				for _, p := range paths {
					res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{
						Action: permissions.FileSystemWrite,
						Path:   p,
					})
				}

			case "touch", "mkdir":
				// FileSystemWrite operations
				paths := extractArgPaths(n.Args[1:])
				for _, p := range paths {
					res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{
						Action: permissions.FileSystemWrite,
						Path:   p,
					})
				}

			case "curl", "wget":
				// Egress Network operations
				var host string
				for _, arg := range n.Args[1:] {
					val := wordToLiteral(arg)
					if val == "" {
						continue
					}
					if !strings.HasPrefix(val, "-") {
						host = extractHostFromURL(val)
						break
					}
				}
				if host != "" {
					res.Network = append(res.Network, permissions.NetworkPermission{
						Direction: "egress",
						Protocol:  "tcp",
						Host:      host,
					})
				}

			default:
				// Map generic command execution as an ExecutablePermission
				var args []string
				for _, arg := range n.Args[1:] {
					val := wordToLiteral(arg)
					if val != "" {
						args = append(args, val)
					}
				}
				res.Executables = append(res.Executables, permissions.ExecutablePermission{
					Binary: binary,
					Args:   args,
				})
			}

		case *syntax.Redirect:
			// Extract target path from redirect node
			target := wordToLiteral(n.Word)
			if target != "" {
				switch n.Op {
				case syntax.RdrOut, syntax.AppOut:
					// FileSystemWrite operations
					res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{
						Action: permissions.FileSystemWrite,
						Path:   target,
					})
				case syntax.RdrIn:
					// FileSystemRead operations
					res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{
						Action: permissions.FileSystemRead,
						Path:   target,
					})
				}
			}
		}
		return true
	})

	return res, nil
}

// wordToLiteral converts a syntax.Word to its literal string representation,
// returning empty string if it contains dynamic parts (variables, substitutions).
func wordToLiteral(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range w.Parts {
		if lit, ok := part.(*syntax.Lit); ok {
			sb.WriteString(lit.Value)
		} else {
			return ""
		}
	}
	return sb.String()
}

// extractArgPaths extracts literal file path arguments, ignoring flags starting with '-'.
func extractArgPaths(words []*syntax.Word) []string {
	var paths []string
	for _, w := range words {
		val := wordToLiteral(w)
		if val != "" && !strings.HasPrefix(val, "-") {
			paths = append(paths, val)
		}
	}
	return paths
}

// extractHostFromURL extracts the host name (e.g. example.com) from a raw URL string.
func extractHostFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	// Strip protocol prefix
	if idx := strings.Index(rawURL, "://"); idx != -1 {
		rawURL = rawURL[idx+3:]
	}
	// Strip path suffix
	if idx := strings.Index(rawURL, "/"); idx != -1 {
		rawURL = rawURL[:idx]
	}
	// Strip port suffix
	if idx := strings.Index(rawURL, ":"); idx != -1 {
		rawURL = rawURL[:idx]
	}
	return rawURL
}
