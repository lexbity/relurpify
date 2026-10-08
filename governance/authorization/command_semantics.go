package authorization

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// maxUnwrapDepth bounds the transparent-wrapper unwrapping chain. Beyond it the
// command is treated as opaque (ask), so a crafted nesting cannot exhaust the
// stack or evade the check by depth.
const maxUnwrapDepth = 5

// LiftedPermissions aggregates all virtual operations statically extracted
// from a shell command string or an argv vector.
type LiftedPermissions struct {
	FileSystem  []permissions.FileSystemPermission
	Executables []permissions.ExecutablePermission
	Network     []permissions.NetworkPermission
	HasDynamic  bool
}

// merge folds another lifted set into res.
func (res *LiftedPermissions) merge(other *LiftedPermissions) {
	if res == nil || other == nil {
		return
	}
	res.FileSystem = append(res.FileSystem, other.FileSystem...)
	res.Executables = append(res.Executables, other.Executables...)
	res.Network = append(res.Network, other.Network...)
	res.HasDynamic = res.HasDynamic || other.HasDynamic
}

// UnwrapStep records one transparent-wrapper unwrapping.
type UnwrapStep struct {
	Wrapper string
	Depth   int
}

// LiftShellCommand parses an arbitrary POSIX/bash command string and walks its
// AST to statically lift low-level commands to high-level virtual permissions.
// A parse error is returned to the caller (which must escalate to ask); it is
// never swallowed.
func LiftShellCommand(cmdStr string) (*LiftedPermissions, error) {
	res := &LiftedPermissions{}
	if err := liftShellString(cmdStr, res, 0); err != nil {
		return nil, err
	}
	return res, nil
}

// liftShellString parses and lifts one shell command string at the given
// wrapper depth.
func liftShellString(cmdStr string, res *LiftedPermissions, depth int) error {
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return nil
	}
	if depth > maxUnwrapDepth {
		res.HasDynamic = true
		return nil
	}

	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(cmdStr), "")
	if err != nil {
		return err
	}

	syntax.Walk(file, func(node syntax.Node) bool {
		if node == nil {
			return true
		}
		switch n := node.(type) {
		case *syntax.CmdSubst:
			res.HasDynamic = true

		case *syntax.CallExpr:
			liftCallExpr(n, res, depth)

		case *syntax.Redirect:
			target, ok := literalWord(n.Word)
			if !ok || target == "" {
				return true
			}
			switch n.Op {
			case syntax.RdrOut, syntax.AppOut:
				res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{
					Action: permissions.FileSystemWrite,
					Path:   target,
				})
			case syntax.RdrIn:
				res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{
					Action: permissions.FileSystemRead,
					Path:   target,
				})
			}
		}
		return true
	})
	return nil
}

// liftCallExpr lifts one parsed command invocation.
func liftCallExpr(n *syntax.CallExpr, res *LiftedPermissions, depth int) {
	if len(n.Args) == 0 {
		return
	}
	argv := make([]string, 0, len(n.Args))
	for _, word := range n.Args {
		value, ok := literalWord(word)
		if !ok {
			// A dynamic argument (variable, substitution) cannot be analyzed;
			// escalate rather than skip the checks (P-3).
			res.HasDynamic = true
			return
		}
		argv = append(argv, value)
	}
	if argv[0] == "" {
		res.HasDynamic = true
		return
	}
	_ = liftCommand(argv, res, depth)
}

// liftCommand lifts one argv vector, recursing through transparent wrappers,
// nested shell strings, and flagging opaque constructors.
func liftCommand(argv []string, res *LiftedPermissions, depth int) error {
	if res == nil || len(argv) == 0 {
		return nil
	}
	if depth > maxUnwrapDepth {
		res.HasDynamic = true
		return nil
	}
	binary := commandBase(argv[0])
	args := argv[1:]

	if isOpaqueConstructor(binary, args) {
		res.HasDynamic = true
		res.Executables = append(res.Executables, permissions.ExecutablePermission{Binary: binary, Args: append([]string(nil), args...)})
		return nil
	}
	if isShellBinary(binary) {
		if cmdStr, ok := shellCommandString(args); ok {
			res.Executables = append(res.Executables, permissions.ExecutablePermission{Binary: binary, Args: append([]string(nil), args...)})
			return liftShellString(cmdStr, res, depth+1)
		}
	}
	if inner, ok := stripArgvWrapper(binary, args); ok {
		res.Executables = append(res.Executables, permissions.ExecutablePermission{Binary: binary, Args: append([]string(nil), args...)})
		return liftCommand(inner, res, depth+1)
	}

	liftPlain(binary, args, res)
	return nil
}

// liftPlain applies the per-binary command→permission mapping and output
// carriers for a non-wrapper, non-opaque command.
func liftPlain(binary string, args []string, res *LiftedPermissions) {
	if res == nil {
		return
	}
	liftOutputCarriers(binary, args, res)

	switch binary {
	case "eval":
		res.HasDynamic = true

	case "cat", "head", "tail", "less", "more", "bat":
		res.addReads(literalPaths(args))

	case "rm", "rmdir", "shred", "unlink":
		res.addDeletes(literalPaths(args))

	case "cp", "mv":
		paths := literalPaths(args)
		if len(paths) > 0 {
			res.addReads(paths[:len(paths)-1])
			res.addWrites(paths[len(paths)-1:])
		}

	case "sed":
		isInPlace := false
		var paths []string
		for _, arg := range args {
			if arg == "" {
				continue
			}
			if arg == "-i" || arg == "--in-place" || strings.HasPrefix(arg, "-i") {
				isInPlace = true
			} else if !strings.HasPrefix(arg, "-") {
				paths = append(paths, arg)
			}
		}
		if len(paths) > 0 {
			target := paths[len(paths)-1]
			if isInPlace {
				res.addWrites([]string{target})
			} else {
				res.addReads([]string{target})
			}
		}

	case "tee":
		res.addWrites(literalPaths(args))

	case "touch", "mkdir":
		res.addWrites(literalPaths(args))

	case "curl", "wget":
		if host := firstNetworkArg(args); host != "" {
			if h := extractHostFromURL(host); h != "" {
				res.Network = append(res.Network, permissions.NetworkPermission{
					Direction: "egress",
					Protocol:  "tcp",
					Host:      h,
				})
			}
		}
	}

	// Every command contributes its executable permission.
	res.Executables = append(res.Executables, permissions.ExecutablePermission{
		Binary: binary,
		Args:   append([]string(nil), args...),
	})
}

// outputCarriers maps a binary to the flags that write a file, so the write is
// lifted through the same CheckFileAccess path as a shell redirect.
func liftOutputCarriers(binary string, args []string, res *LiftedPermissions) {
	addWrite := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || strings.HasPrefix(path, "-") {
			return
		}
		res.addWrites([]string{path})
	}
	switch binary {
	case "curl":
		if v, ok := flagValue(args, "-o", "--output"); ok {
			addWrite(v)
		}
	case "wget":
		if v, ok := flagValue(args, "-O", "--output-document"); ok {
			addWrite(v)
		}
	case "dd":
		for _, arg := range args {
			if strings.HasPrefix(arg, "of=") {
				addWrite(strings.TrimPrefix(arg, "of="))
			}
		}
	case "install":
		paths := literalPaths(args)
		if len(paths) > 0 {
			addWrite(paths[len(paths)-1])
		}
	case "truncate":
		for _, path := range literalPaths(args) {
			addWrite(path)
		}
	}
}

func (res *LiftedPermissions) addReads(paths []string) {
	for _, p := range paths {
		res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{Action: permissions.FileSystemRead, Path: p})
	}
}

func (res *LiftedPermissions) addWrites(paths []string) {
	for _, p := range paths {
		res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{Action: permissions.FileSystemWrite, Path: p})
	}
}

func (res *LiftedPermissions) addDeletes(paths []string) {
	for _, p := range paths {
		res.FileSystem = append(res.FileSystem, permissions.FileSystemPermission{Action: permissions.FileSystemDelete, Path: p})
	}
}

// UnwrapCommand iteratively strips transparent wrappers from argv, returning
// the inner argv vectors (outer-first), the steps taken, and whether an opaque
// constructor or a depth overflow was encountered. Shell wrappers are returned
// as their own argv vector so the caller can re-lift the shell string.
func UnwrapCommand(argv []string) ([][]string, []UnwrapStep, bool) {
	var inner [][]string
	var steps []UnwrapStep
	dynamic := false

	cur := append([]string(nil), argv...)
	for depth := 0; len(cur) > 0; depth++ {
		if depth >= maxUnwrapDepth {
			dynamic = true
			break
		}
		binary := commandBase(cur[0])
		args := cur[1:]

		if isOpaqueConstructor(binary, args) {
			dynamic = true
			break
		}
		if isShellBinary(binary) {
			if _, ok := shellCommandString(args); ok {
				inner = append(inner, cur)
				steps = append(steps, UnwrapStep{Wrapper: binary, Depth: depth})
			}
			break
		}
		next, ok := stripArgvWrapper(binary, args)
		if !ok {
			break
		}
		steps = append(steps, UnwrapStep{Wrapper: binary, Depth: depth})
		cur = next
		inner = append(inner, append([]string(nil), next...))
	}
	return inner, steps, dynamic
}

// commandBase lowercases the basename of a command token.
func commandBase(binary string) string {
	return strings.ToLower(filepath.Base(strings.TrimSpace(binary)))
}

// isShellBinary reports whether the command is a POSIX shell.
func isShellBinary(binary string) bool {
	switch binary {
	case "sh", "bash", "zsh", "dash":
		return true
	default:
		return false
	}
}

// shellCommandString returns the command string of a shell `-c` invocation.
// It accepts any combined short-flag cluster containing `c` (e.g. -c, -lc,
// -cl, -ic).
func shellCommandString(args []string) (string, bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if len(arg) < 2 || arg[0] != '-' || arg == "--" {
			continue
		}
		if strings.HasPrefix(arg, "--") {
			continue
		}
		if !strings.ContainsRune(arg[1:], 'c') {
			continue
		}
		if i+1 < len(args) {
			return args[i+1], true
		}
		return "", false
	}
	return "", false
}

// isOpaqueConstructor reports whether a command's behavior cannot be statically
// analyzed (eval-like, code-bearing flags, or arbitrary command execution).
func isOpaqueConstructor(binary string, args []string) bool {
	switch binary {
	case "xargs", "sudo", "doas", "su", "watch":
		return true
	case "find":
		for _, arg := range args {
			switch arg {
			case "-exec", "-execdir", "-ok", "-okdir":
				return true
			}
		}
	case "awk", "gawk", "mawk":
		for _, arg := range args {
			if strings.Contains(arg, "system(") {
				return true
			}
		}
	case "perl":
		for _, arg := range args {
			if arg == "-e" || arg == "-E" || arg == "--eval" || (strings.HasPrefix(arg, "-e") && len(arg) > 2) {
				return true
			}
		}
	case "python", "python2", "python3", "node", "nodejs", "ruby", "php":
		for _, arg := range args {
			switch arg {
			case "-c", "-e", "--eval", "-p", "-r":
				return true
			}
		}
	}
	return false
}

// stripArgvWrapper returns the inner argv of a transparent wrapper, or false
// when the command is not a recognized wrapper.
func stripArgvWrapper(binary string, args []string) ([]string, bool) {
	var out []string
	switch binary {
	case "env":
		out = stripEnv(args)
	case "nice":
		out = stripNice(args)
	case "timeout":
		out = stripTimeout(args)
	case "stdbuf":
		out = stripStdbuf(args)
	case "nohup", "setsid":
		out = skipLeadingFlags(args)
	default:
		return nil, false
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func stripEnv(args []string) []string {
	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "--":
			return args[i+1:]
		case arg == "-i" || arg == "--ignore-environment" || arg == "-0" || arg == "--null":
			i++
		case arg == "-u" || arg == "--unset" || arg == "-C" || arg == "--chdir" || arg == "-S" || arg == "--split-string":
			i += 2
		case strings.HasPrefix(arg, "--unset=") || strings.HasPrefix(arg, "--chdir=") || strings.HasPrefix(arg, "--split-string="):
			i++
		case strings.HasPrefix(arg, "-") && arg != "-":
			i++
		case strings.Contains(arg, "=") && !strings.HasPrefix(arg, "="):
			i++
		default:
			return args[i:]
		}
	}
	return nil
}

func stripNice(args []string) []string {
	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-n" || arg == "--adjustment":
			i += 2
		case strings.HasPrefix(arg, "--adjustment="):
			i++
		case len(arg) > 1 && arg[0] == '-' && allDigits(arg[1:]):
			i++
		default:
			return args[i:]
		}
	}
	return nil
}

func stripTimeout(args []string) []string {
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		switch args[i] {
		case "-s", "--signal", "-k", "--kill-after":
			i += 2
		default:
			i++
		}
	}
	if i >= len(args) {
		return nil
	}
	i++ // duration token
	if i >= len(args) {
		return nil
	}
	return args[i:]
}

func stripStdbuf(args []string) []string {
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			return args[i+1:]
		}
		if strings.HasPrefix(arg, "--") {
			i++
			continue
		}
		if len(arg) >= 2 && arg[0] == '-' && (arg[1] == 'i' || arg[1] == 'o' || arg[1] == 'e') {
			if len(arg) == 2 {
				i += 2
			} else {
				i++
			}
			continue
		}
		return args[i:]
	}
	return nil
}

func skipLeadingFlags(args []string) []string {
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		if args[i] == "--" {
			i++
			break
		}
		i++
	}
	return args[i:]
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// literalWord returns a word's literal value and whether it is fully literal.
// Single- and double-quoted strings whose parts are all literal count as
// literal; parameter expansions, substitutions, and arithmetic do not.
func literalWord(w *syntax.Word) (string, bool) {
	if w == nil {
		return "", true
	}
	var sb strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			sb.WriteString(p.Value)
		case *syntax.SglQuoted:
			if p.Dollar {
				return "", false
			}
			sb.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return "", false
				}
				sb.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	return sb.String(), true
}

// literalPaths returns non-flag arguments.
func literalPaths(args []string) []string {
	var paths []string
	for _, arg := range args {
		if arg != "" && !strings.HasPrefix(arg, "-") {
			paths = append(paths, arg)
		}
	}
	return paths
}

// firstNetworkArg returns the first non-flag argument that is not the value of
// an output-carrier flag, so `curl -o out.txt URL` lifts the URL host, not the
// output filename.
func firstNetworkArg(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-o" || arg == "--output" || arg == "-O" || arg == "--output-document":
			i++
		case strings.HasPrefix(arg, "--output=") || strings.HasPrefix(arg, "--output-document="):
		case strings.HasPrefix(arg, "-o") && len(arg) > 2 && !strings.HasPrefix(arg, "--"):
		case strings.HasPrefix(arg, "-O") && len(arg) > 2 && !strings.HasPrefix(arg, "--"):
		case arg != "" && !strings.HasPrefix(arg, "-"):
			return arg
		}
	}
	return ""
}

// flagValue finds the value of a short/long flag in args.
func flagValue(args []string, short, long string) (string, bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == short || arg == long {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", false
		}
		if long != "" && strings.HasPrefix(arg, long+"=") {
			return strings.TrimPrefix(arg, long+"="), true
		}
		if short != "" && len(arg) > len(short) && strings.HasPrefix(arg, short) && !strings.HasPrefix(arg, "--") {
			return arg[len(short):], true
		}
	}
	return "", false
}

// extractHostFromURL extracts the host name (e.g. example.com) from a raw URL string.
func extractHostFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if idx := strings.Index(rawURL, "://"); idx != -1 {
		rawURL = rawURL[idx+3:]
	}
	if idx := strings.Index(rawURL, "/"); idx != -1 {
		rawURL = rawURL[:idx]
	}
	if idx := strings.Index(rawURL, ":"); idx != -1 {
		rawURL = rawURL[:idx]
	}
	return rawURL
}
