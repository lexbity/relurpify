package authorization

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- test helpers ----

// basePermSet builds a minimal valid core.PermissionSet anchored at base.
func basePermSet(base string) *core.PermissionSet {
	path := "/**"
	if base != "" {
		path = base + "/**"
	}
	return &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: path},
		},
	}
}

// newTestPM creates a PermissionManager, failing immediately on error.
func newTestPM(t *testing.T, base string, perms *core.PermissionSet) *PermissionManager {
	t.Helper()
	m, err := NewPermissionManager(base, perms, nil, nil)
	require.NoError(t, err)
	return m
}

// autoApproveHITL immediately grants every HITL request.
type autoApproveHITL struct{ calls int }

func (a *autoApproveHITL) RequestPermission(_ context.Context, req PermissionRequest) (*PermissionGrant, error) {
	a.calls++
	return &PermissionGrant{
		ID:         "auto-grant",
		Permission: req.Permission,
		Scope:      GrantScopeSession,
	}, nil
}

type recordingApproveHITL struct {
	calls    int
	requests []PermissionRequest
}

func (r *recordingApproveHITL) RequestPermission(_ context.Context, req PermissionRequest) (*PermissionGrant, error) {
	r.calls++
	r.requests = append(r.requests, req)
	return &PermissionGrant{
		ID:         "recording-grant",
		Permission: req.Permission,
		Scope:      GrantScopeSession,
	}, nil
}

// stubPermTool satisfies core.Tool for permission authorization tests.
type stubPermTool struct {
	name  string
	perms *core.PermissionSet
}

func (s stubPermTool) Name() string                     { return s.name }
func (s stubPermTool) Description() string              { return "stub" }
func (s stubPermTool) Category() string                 { return "test" }
func (s stubPermTool) Parameters() []core.ToolParameter { return nil }
func (s stubPermTool) Execute(_ context.Context, _ *core.Context, _ map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (s stubPermTool) IsAvailable(_ context.Context, _ *core.Context) bool { return true }
func (s stubPermTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: s.perms}
}
func (s stubPermTool) Tags() []string { return nil }

// ---- matchGlob ----

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		// bare ** matches everything, including empty
		{name: "bare ** matches any path", pattern: "**", value: "/any/deep/path", want: true},
		{name: "bare ** matches empty string", pattern: "**", value: "", want: true},

		// exact matches (no wildcard — filepath.Match semantics)
		{name: "exact match", pattern: "/ws/main.go", value: "/ws/main.go", want: true},
		{name: "exact mismatch", pattern: "/ws/main.go", value: "/ws/other.go", want: false},

		// dot in filename must be literal, not a regex wildcard
		{name: "literal dot in pattern", pattern: "/ws/file.go", value: "/ws/file.go", want: true},
		{name: "literal dot not regex wildcard", pattern: "/ws/file.go", value: "/ws/fileXgo", want: false},

		// single-segment * does not cross directory separators
		{name: "* matches within segment", pattern: "/ws/*.go", value: "/ws/main.go", want: true},
		{name: "* does not cross slash", pattern: "/ws/*.go", value: "/ws/sub/main.go", want: false},

		// ? matches exactly one non-separator character
		{name: "? matches single char", pattern: "/ws/?.go", value: "/ws/a.go", want: true},
		{name: "? does not match two chars", pattern: "/ws/?.go", value: "/ws/ab.go", want: false},

		// /** — the base directory itself is matched via the special-case shortcut
		{name: "/** matches the directory itself", pattern: "/ws/**", value: "/ws", want: true},
		{name: "/** matches shallow file", pattern: "/ws/**", value: "/ws/a.go", want: true},
		{name: "/** matches deeply nested path", pattern: "/ws/**", value: "/ws/a/b/c.go", want: true},
		{name: "/** does not match sibling", pattern: "/ws/**", value: "/other/path", want: false},

		// ** embedded in pattern requires at least one path segment between anchors
		{name: "**/file.go matches with one segment", pattern: "/root/**/file.go", value: "/root/a/file.go", want: true},
		{name: "**/file.go matches with deep nesting", pattern: "/root/**/file.go", value: "/root/a/b/c/file.go", want: true},
		{name: "**/file.go requires separator — no direct sibling", pattern: "/root/**/file.go", value: "/root/file.go", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, matchGlob(tc.pattern, tc.value))
		})
	}
}

// ---- globToRegex ----

func TestGlobToRegex(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		want    string
	}{
		{name: "** becomes .*", pattern: "**", want: "^.*$"},
		{name: "* becomes [^/]*", pattern: "*.go", want: "^[^/]*\\.go$"},
		{name: "? becomes .", pattern: "a?.go", want: "^a.\\.go$"},
		{name: "dot is escaped", pattern: "file.go", want: "^file\\.go$"},
		{name: "path with embedded **", pattern: "/root/**/file.go", want: "^/root/.*/file\\.go$"},
		{name: "plus is escaped", pattern: "a+b", want: "^a\\+b$"},
		{name: "parentheses are escaped", pattern: "a(b)", want: "^a\\(b\\)$"},
		{name: "brackets are escaped", pattern: "a[b]", want: "^a\\[b\\]$"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, globToRegex(tc.pattern))
		})
	}
}

// ---- matchArgs ----

func TestMatchArgs(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		args     []string
		want     bool
	}{
		// empty pattern list allows any args
		{name: "empty patterns allow any args", patterns: []string{}, args: []string{"anything"}, want: true},

		// a single bare * allows everything
		{name: "single * wildcard allows all", patterns: []string{"*"}, args: []string{"a", "b", "c"}, want: true},

		// exact matching
		{name: "exact args match", patterns: []string{"test", "-v"}, args: []string{"test", "-v"}, want: true},
		{name: "extra arg without trailing wildcard fails", patterns: []string{"test"}, args: []string{"test", "-v"}, want: false},
		{name: "missing arg fails", patterns: []string{"test", "-v"}, args: []string{"test"}, want: false},

		// * in a non-terminal position matches any single value
		{name: "middle * matches any value", patterns: []string{"build", "*", "--output=bin"}, args: []string{"build", "main.go", "--output=bin"}, want: true},
		// trailing * allows zero or more additional args
		{name: "trailing * satisfied with zero extra args", patterns: []string{"build", "*"}, args: []string{"build"}, want: true},

		// trailing * allows additional arguments
		{name: "trailing * allows extra args", patterns: []string{"go", "build", "*"}, args: []string{"go", "build", "-o", "out", "./..."}, want: true},
		{name: "trailing * is satisfied with no extra args", patterns: []string{"go", "*"}, args: []string{"go"}, want: true},

		// --flag* prefix matching
		{name: "--flag* prefix matches", patterns: []string{"--output=*"}, args: []string{"--output=bin"}, want: true},
		{name: "--flag* prefix mismatch fails", patterns: []string{"--output=*"}, args: []string{"--verbose"}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, matchArgs(tc.patterns, tc.args))
		})
	}
}

// ---- matchEnv ----

func TestMatchEnv(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		env      []string
		want     bool
	}{
		// empty pattern list succeeds regardless of env
		{name: "empty patterns match any env", patterns: []string{}, env: []string{"X=1"}, want: true},

		// exact key=value
		{name: "exact key=value match", patterns: []string{"GOPATH=/home/go"}, env: []string{"GOPATH=/home/go"}, want: true},
		{name: "wrong value fails", patterns: []string{"GOPATH=/other"}, env: []string{"GOPATH=/home/go"}, want: false},
		{name: "missing key fails", patterns: []string{"GOPATH=/home/go"}, env: []string{"HOME=/root"}, want: false},

		// * value wildcard accepts any value for the key
		{name: "* value accepts any value", patterns: []string{"GOPATH=*"}, env: []string{"GOPATH=/any/value"}, want: true},

		// all patterns must pass
		{name: "all patterns satisfied", patterns: []string{"A=1", "B=2"}, env: []string{"A=1", "B=2", "C=3"}, want: true},
		{name: "one mismatch fails all", patterns: []string{"A=1", "B=3"}, env: []string{"A=1", "B=2"}, want: false},

		// malformed pattern without = is silently skipped (does not fail)
		{name: "malformed pattern without = is skipped", patterns: []string{"NOEQUALS"}, env: []string{"X=1"}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, matchEnv(tc.patterns, tc.env))
		})
	}
}

// ---- expandWorkspacePlaceholder ----

func TestExpandWorkspacePlaceholder(t *testing.T) {
	cases := []struct {
		name      string
		workspace string
		pattern   string
		want      string
	}{
		{name: "empty pattern returns empty", workspace: "/ws", pattern: "", want: ""},
		{name: "${workspace} replaced", workspace: "/ws", pattern: "${workspace}/**", want: "/ws/**"},
		{name: "${WORKSPACE} replaced", workspace: "/ws", pattern: "${WORKSPACE}/**", want: "/ws/**"},
		{name: "{{workspace}} replaced", workspace: "/ws", pattern: "{{workspace}}/**", want: "/ws/**"},
		{name: "{{WORKSPACE}} replaced", workspace: "/ws", pattern: "{{WORKSPACE}}/**", want: "/ws/**"},
		{name: "absolute path returned unchanged", workspace: "/ws", pattern: "/abs/path/**", want: "/abs/path/**"},
		{name: "relative path joined with workspace", workspace: "/ws", pattern: "src/**", want: "/ws/src/**"},
		{name: "./ prefix stripped before join", workspace: "/ws", pattern: "./src/**", want: "/ws/src/**"},
		{name: "empty workspace leaves relative pattern as-is", workspace: "", pattern: "src/**", want: "src/**"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, expandWorkspacePlaceholder(tc.workspace, tc.pattern))
		})
	}
}

func TestPermissionManagerLogRedactsSensitiveMetadata(t *testing.T) {
	audit := core.NewInMemoryAuditLogger(8)
	pm, err := NewPermissionManager(t.TempDir(), basePermSet(""), audit, nil)
	require.NoError(t, err)

	pm.log(context.Background(), "agent-1", core.PermissionDescriptor{
		Type:     core.PermissionTypeCapability,
		Action:   "tool:test",
		Resource: "agent-1",
	}, "allowed", map[string]interface{}{
		"token":         "secret-token",
		"authorization": "Bearer abc",
		"plain":         "ok",
	})

	records, err := audit.Query(context.Background(), core.AuditQuery{AgentID: "agent-1"})
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "[REDACTED]", records[0].Metadata["token"])
	require.Equal(t, "[REDACTED]", records[0].Metadata["authorization"])
	require.Equal(t, "ok", records[0].Metadata["plain"])
}

// ---- normalizePath ----

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		name      string
		base      string
		path      string
		wantPath  string
		wantError bool
	}{
		{name: "relative path joined with basePath", base: "/workspace", path: "src/main.go", wantPath: "/workspace/src/main.go"},
		{name: "nested relative joined correctly", base: "/ws", path: "a/b/c.go", wantPath: "/ws/a/b/c.go"},
		{name: "absolute path returned as-is", base: "/workspace", path: "/etc/hosts", wantPath: "/etc/hosts"},
		{name: "traversal via leading ../ rejected", base: "/workspace", path: "../etc/passwd", wantError: true},
		{name: "traversal via nested dirs rejected", base: "/workspace", path: "a/../../etc", wantError: true},
		{name: "empty basePath returns relative unchanged", base: "", path: "src/main.go", wantPath: "src/main.go"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestPM(t, tc.base, basePermSet(tc.base))
			got, err := m.normalizePath(tc.path)
			if tc.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "path traversal")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantPath, got)
			}
		})
	}
}

// ---- PermissionGrant.Expired ----

func TestPermissionGrantExpired(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name  string
		grant *PermissionGrant
		clock time.Time
		want  bool
	}{
		{name: "nil grant is always expired", grant: nil, clock: now, want: true},
		{name: "zero ExpiresAt never expires", grant: &PermissionGrant{}, clock: now, want: false},
		{name: "future expiry is not expired", grant: &PermissionGrant{ExpiresAt: now.Add(time.Hour)}, clock: now, want: false},
		{name: "past expiry is expired", grant: &PermissionGrant{ExpiresAt: now.Add(-time.Second)}, clock: now, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.grant.Expired(tc.clock))
		})
	}
}

func TestGrantManualZeroDurationDoesNotExpire(t *testing.T) {
	grant := GrantManual(core.PermissionDescriptor{
		Type:     core.PermissionTypeHITL,
		Action:   "capability:tool:file_write",
		Resource: "agent",
	}, "tester", GrantScopeSession, 0)
	if grant == nil {
		t.Fatal("expected grant")
	}
	assert.True(t, grant.ExpiresAt.IsZero())
	assert.False(t, grant.Expired(time.Now().Add(time.Hour)))
}

// ---- effectiveDefaultPolicy ----

func TestEffectiveDefaultPolicy(t *testing.T) {
	m := newTestPM(t, "/ws", basePermSet("/ws"))

	// zero-value defaults to Ask
	assert.Equal(t, core.AgentPermissionAsk, m.effectiveDefaultPolicy())

	m.SetDefaultPolicy(core.AgentPermissionAllow)
	assert.Equal(t, core.AgentPermissionAllow, m.effectiveDefaultPolicy())

	m.SetDefaultPolicy(core.AgentPermissionDeny)
	assert.Equal(t, core.AgentPermissionDeny, m.effectiveDefaultPolicy())

	m.SetDefaultPolicy(core.AgentPermissionAsk)
	assert.Equal(t, core.AgentPermissionAsk, m.effectiveDefaultPolicy())
}

// ---- collectUndeclared ----

func TestCollectUndeclared(t *testing.T) {
	declared := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/ws/**"},
		},
		Executables: []core.ExecutablePermission{
			{Binary: "go"},
		},
		Network: []core.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
		},
	}
	m := newTestPM(t, "/ws", declared)

	t.Run("all declared — nothing missing", func(t *testing.T) {
		reqs := &core.PermissionSet{
			FileSystem:  []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/ws/**"}},
			Executables: []core.ExecutablePermission{{Binary: "go"}},
		}
		assert.Empty(t, m.collectUndeclared(reqs))
	})

	t.Run("undeclared filesystem path reported", func(t *testing.T) {
		reqs := &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/etc/**"}},
		}
		missing := m.collectUndeclared(reqs)
		require.Len(t, missing, 1)
		assert.Contains(t, missing[0], "fs")
		assert.Contains(t, missing[0], "/etc/**")
	})

	t.Run("undeclared binary reported", func(t *testing.T) {
		reqs := &core.PermissionSet{
			FileSystem:  []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/ws/**"}},
			Executables: []core.ExecutablePermission{{Binary: "npm"}},
		}
		missing := m.collectUndeclared(reqs)
		require.Len(t, missing, 1)
		assert.Contains(t, missing[0], "npm")
	})

	t.Run("multiple undeclared items all reported", func(t *testing.T) {
		reqs := &core.PermissionSet{
			FileSystem:  []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/etc/**"}},
			Executables: []core.ExecutablePermission{{Binary: "npm"}},
		}
		missing := m.collectUndeclared(reqs)
		assert.Len(t, missing, 2)
	})
}

// ---- AuthorizeTool — default policy three-way switch ----

func TestAuthorizeToolDefaultPolicies(t *testing.T) {
	ctx := context.Background()

	// agent declares read access only
	agentPerms := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/ws/**"},
		},
	}

	// tool requesting write access to /etc — not in agent manifest
	undeclaredTool := stubPermTool{
		name: "secretive",
		perms: &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{
				{Action: core.FileSystemWrite, Path: "/etc/**"},
			},
		},
	}

	t.Run("Deny blocks undeclared tool", func(t *testing.T) {
		m := newTestPM(t, "/ws", agentPerms)
		m.SetDefaultPolicy(core.AgentPermissionDeny)
		err := m.AuthorizeTool(ctx, "agent", undeclaredTool, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds agent permissions")
	})

	t.Run("Allow passes undeclared tool without HITL", func(t *testing.T) {
		m := newTestPM(t, "/ws", agentPerms)
		m.SetDefaultPolicy(core.AgentPermissionAllow)
		require.NoError(t, m.AuthorizeTool(ctx, "agent", undeclaredTool, nil))
	})

	t.Run("Ask with HITL provider routes to HITL", func(t *testing.T) {
		hitl := &autoApproveHITL{}
		m, err := NewPermissionManager("/ws", agentPerms, nil, hitl)
		require.NoError(t, err)
		m.SetDefaultPolicy(core.AgentPermissionAsk)
		require.NoError(t, m.AuthorizeTool(ctx, "agent", undeclaredTool, nil))
		assert.Equal(t, 1, hitl.calls, "HITL must be called exactly once")
	})

	t.Run("Ask with no HITL provider denies", func(t *testing.T) {
		m := newTestPM(t, "/ws", agentPerms) // nil HITL
		m.SetDefaultPolicy(core.AgentPermissionAsk)
		require.Error(t, m.AuthorizeTool(ctx, "agent", undeclaredTool, nil))
	})

	t.Run("declared tool always passes regardless of default policy", func(t *testing.T) {
		declaredTool := stubPermTool{
			name: "reader",
			perms: &core.PermissionSet{
				FileSystem: []core.FileSystemPermission{
					{Action: core.FileSystemRead, Path: "/ws/**"},
				},
			},
		}
		m := newTestPM(t, "/ws", agentPerms)
		m.SetDefaultPolicy(core.AgentPermissionDeny)
		require.NoError(t, m.AuthorizeTool(ctx, "agent", declaredTool, nil))
	})
}

func TestRegisterTaskGrantRejectsWildcard(t *testing.T) {
	m := newTestPM(t, "/ws", basePermSet("/ws"))
	err := m.RegisterTaskGrant("run-1", []string{"*"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "wildcard")
}

func TestAuthorizeToolAllowsMatchingTaskGrant(t *testing.T) {
	m := newTestPM(t, "/ws", basePermSet("/ws"))
	require.NoError(t, m.RegisterTaskGrant("run-1", []string{"safe", "review"}))

	tool := stubPermTool{
		name: "tagged",
		perms: &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{
				{Action: core.FileSystemWrite, Path: "/etc/**"},
			},
		},
	}
	ctx := core.WithTaskContext(context.Background(), core.TaskContext{ID: "run-1"})

	toolWithTags := struct{ stubPermTool }{tool}
	require.NoError(t, m.AuthorizeTool(ctx, "agent", taggedTool{Tool: toolWithTags, tags: []string{"safe", "review"}}, nil))
}

// TestAuthorizeToolTaskGrantAnyTagAllows verifies that a tool is granted when
// at least one of its tags appears in the task grant — not all tags must match.
func TestAuthorizeToolTaskGrantAnyTagAllows(t *testing.T) {
	m := newTestPM(t, "/ws", basePermSet("/ws"))
	require.NoError(t, m.RegisterTaskGrant("run-1", []string{"safe"}))
	m.SetDefaultPolicy(core.AgentPermissionDeny)

	// Tool has tags ["safe", "review"]; grant only covers "safe" — should allow.
	tool := taggedTool{
		Tool: stubPermTool{
			name: "tagged",
			perms: &core.PermissionSet{
				FileSystem: []core.FileSystemPermission{
					{Action: core.FileSystemWrite, Path: "/etc/**"},
				},
			},
		},
		tags: []string{"safe", "review"},
	}
	ctx := core.WithTaskContext(context.Background(), core.TaskContext{ID: "run-1"})
	require.NoError(t, m.AuthorizeTool(ctx, "agent", tool, nil))
}

// TestAuthorizeToolTaskGrantDeniesNoMatchingTag verifies that a tool is denied
// when none of its tags appear in the task grant.
func TestAuthorizeToolTaskGrantDeniesNoMatchingTag(t *testing.T) {
	m := newTestPM(t, "/ws", basePermSet("/ws"))
	require.NoError(t, m.RegisterTaskGrant("run-1", []string{"safe"}))
	m.SetDefaultPolicy(core.AgentPermissionDeny)

	// Tool has only "review" which is not in the grant — should deny.
	tool := taggedTool{
		Tool: stubPermTool{
			name: "tagged",
			perms: &core.PermissionSet{
				FileSystem: []core.FileSystemPermission{
					{Action: core.FileSystemWrite, Path: "/etc/**"},
				},
			},
		},
		tags: []string{"review"},
	}
	ctx := core.WithTaskContext(context.Background(), core.TaskContext{ID: "run-1"})
	require.Error(t, m.AuthorizeTool(ctx, "agent", tool, nil))
}

type taggedTool struct {
	core.Tool
	tags []string
}

func (t taggedTool) Tags() []string { return append([]string{}, t.tags...) }

// ---- CheckFileAccess — default policy three-way switch ----

func TestCheckFileAccessDefaultPolicies(t *testing.T) {
	ctx := context.Background()

	// declared: read only under /ws/src
	declared := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/ws/src/**"},
		},
	}

	t.Run("declared path succeeds", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		require.NoError(t, m.CheckFileAccess(ctx, "agent", core.FileSystemRead, "/ws/src/main.go"))
	})

	t.Run("Deny blocks undeclared path", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		m.SetDefaultPolicy(core.AgentPermissionDeny)
		require.Error(t, m.CheckFileAccess(ctx, "agent", core.FileSystemRead, "/ws/other/file.go"))
	})

	t.Run("Allow passes undeclared path", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		m.SetDefaultPolicy(core.AgentPermissionAllow)
		require.NoError(t, m.CheckFileAccess(ctx, "agent", core.FileSystemRead, "/ws/other/file.go"))
	})

	t.Run("Ask routes undeclared path to HITL", func(t *testing.T) {
		hitl := &autoApproveHITL{}
		m, err := NewPermissionManager("/ws", declared, nil, hitl)
		require.NoError(t, err)
		m.SetDefaultPolicy(core.AgentPermissionAsk)
		require.NoError(t, m.CheckFileAccess(ctx, "agent", core.FileSystemRead, "/ws/other/file.go"))
		assert.Equal(t, 1, hitl.calls)
	})

	t.Run("declared path with HITLRequired triggers approval", func(t *testing.T) {
		hitl := &autoApproveHITL{}
		perms := &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{
				{Action: core.FileSystemWrite, Path: "/ws/**", HITLRequired: true},
			},
		}
		m, err := NewPermissionManager("/ws", perms, nil, hitl)
		require.NoError(t, err)
		require.NoError(t, m.CheckFileAccess(ctx, "agent", core.FileSystemWrite, "/ws/main.go"))
		assert.Equal(t, 1, hitl.calls)
	})

	t.Run("second request on HITLRequired path uses cached grant", func(t *testing.T) {
		hitl := &autoApproveHITL{}
		perms := &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{
				{Action: core.FileSystemWrite, Path: "/ws/**", HITLRequired: true},
			},
		}
		m, err := NewPermissionManager("/ws", perms, nil, hitl)
		require.NoError(t, err)
		require.NoError(t, m.CheckFileAccess(ctx, "agent", core.FileSystemWrite, "/ws/main.go"))
		require.NoError(t, m.CheckFileAccess(ctx, "agent", core.FileSystemWrite, "/ws/main.go"))
		assert.Equal(t, 1, hitl.calls, "cached grant must prevent a second HITL call")
	})

	t.Run("filesystem permission lookup populates cache for match and miss", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		require.NoError(t, m.CheckFileAccess(ctx, "agent", core.FileSystemRead, "/ws/src/main.go"))
		m.SetDefaultPolicy(core.AgentPermissionAllow)
		require.NoError(t, m.CheckFileAccess(ctx, "agent", core.FileSystemRead, "/ws/other/file.go"))

		m.mu.RLock()
		defer m.mu.RUnlock()
		matchKey := string(core.FileSystemRead) + ":/ws/src/main.go"
		missKey := string(core.FileSystemRead) + ":/ws/other/file.go"
		require.Contains(t, m.fsPermCache, matchKey)
		require.Contains(t, m.fsPermCache, missKey)
		require.NotNil(t, m.fsPermCache[matchKey])
		require.Nil(t, m.fsPermCache[missKey])
	})
}

// ---- CheckExecutable — arg and env matching, default policies ----

func TestCheckExecutable(t *testing.T) {
	ctx := context.Background()

	declared := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/ws/**"},
		},
		Executables: []core.ExecutablePermission{
			{Binary: "go", Args: []string{"test", "*"}},
			{Binary: "git"},
			{Binary: "make", HITLRequired: true},
		},
	}

	t.Run("declared binary with matching args passes", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		require.NoError(t, m.CheckExecutable(ctx, "agent", "go", []string{"test", "./..."}, nil))
	})

	t.Run("declared binary with non-matching args rejected", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		err := m.CheckExecutable(ctx, "agent", "go", []string{"run", "main.go"}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "arguments rejected")
	})

	t.Run("declared binary with no arg restriction accepts any args", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		require.NoError(t, m.CheckExecutable(ctx, "agent", "git", []string{"status"}, nil))
		require.NoError(t, m.CheckExecutable(ctx, "agent", "git", []string{"commit", "-m", "msg"}, nil))
	})

	t.Run("declared binary with HITLRequired triggers approval", func(t *testing.T) {
		hitl := &autoApproveHITL{}
		m, err := NewPermissionManager("/ws", declared, nil, hitl)
		require.NoError(t, err)
		require.NoError(t, m.CheckExecutable(ctx, "agent", "make", nil, nil))
		assert.Equal(t, 1, hitl.calls)
	})

	t.Run("undeclared binary + Deny rejected", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		m.SetDefaultPolicy(core.AgentPermissionDeny)
		err := m.CheckExecutable(ctx, "agent", "npm", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not declared")
	})

	t.Run("undeclared binary + Allow passes", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		m.SetDefaultPolicy(core.AgentPermissionAllow)
		require.NoError(t, m.CheckExecutable(ctx, "agent", "npm", nil, nil))
	})

	t.Run("undeclared binary + Ask routes to HITL", func(t *testing.T) {
		hitl := &autoApproveHITL{}
		m, err := NewPermissionManager("/ws", declared, nil, hitl)
		require.NoError(t, err)
		m.SetDefaultPolicy(core.AgentPermissionAsk)
		require.NoError(t, m.CheckExecutable(ctx, "agent", "npm", nil, nil))
		assert.Equal(t, 1, hitl.calls)
	})

	t.Run("executable permission lookup populates cache for match and miss", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		require.NoError(t, m.CheckExecutable(ctx, "agent", "git", []string{"status"}, nil))
		m.SetDefaultPolicy(core.AgentPermissionAllow)
		require.NoError(t, m.CheckExecutable(ctx, "agent", "npm", nil, nil))

		m.mu.RLock()
		defer m.mu.RUnlock()
		require.Contains(t, m.execPermCache, "git")
		require.Contains(t, m.execPermCache, "npm")
		require.NotNil(t, m.execPermCache["git"])
		require.Nil(t, m.execPermCache["npm"])
	})
}

func TestCheckExecutableEnvMatching(t *testing.T) {
	ctx := context.Background()

	declared := &core.PermissionSet{
		Executables: []core.ExecutablePermission{
			{Binary: "python3", Env: []string{"PYTHONPATH=*", "HOME=/root"}},
		},
	}
	m := newTestPM(t, "/ws", declared)

	t.Run("matching env passes", func(t *testing.T) {
		env := []string{"PYTHONPATH=/mylib", "HOME=/root"}
		require.NoError(t, m.CheckExecutable(ctx, "agent", "python3", nil, env))
	})

	t.Run("missing required env key rejected", func(t *testing.T) {
		env := []string{"PYTHONPATH=/mylib"} // HOME missing
		err := m.CheckExecutable(ctx, "agent", "python3", nil, env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "environment rejected")
	})

	t.Run("wrong env value rejected", func(t *testing.T) {
		env := []string{"PYTHONPATH=/mylib", "HOME=/other"}
		err := m.CheckExecutable(ctx, "agent", "python3", nil, env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "environment rejected")
	})
}

func TestAuthorizeCommand(t *testing.T) {
	ctx := context.Background()

	t.Run("empty command rejected", func(t *testing.T) {
		err := AuthorizeCommand(ctx, nil, "agent", nil, CommandAuthorizationRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "command empty")
	})

	t.Run("declared executable with allow bash policy passes", func(t *testing.T) {
		perms := &core.PermissionSet{
			Executables: []core.ExecutablePermission{
				{Binary: "go", Args: []string{"test", "*"}},
			},
		}
		m := newTestPM(t, "/ws", perms)
		spec := &core.AgentRuntimeSpec{
			Bash: core.AgentBashPermissions{
				AllowPatterns: []string{"go test **"},
				Default:       core.AgentPermissionDeny,
			},
		}
		err := AuthorizeCommand(ctx, m, "agent", spec, CommandAuthorizationRequest{
			Command: []string{"go", "test", "./..."},
			Source:  "unit-test",
		})
		require.NoError(t, err)
	})

	t.Run("bash deny pattern blocks declared executable", func(t *testing.T) {
		perms := &core.PermissionSet{
			Executables: []core.ExecutablePermission{
				{Binary: "git"},
			},
		}
		m := newTestPM(t, "/ws", perms)
		spec := &core.AgentRuntimeSpec{
			Bash: core.AgentBashPermissions{
				DenyPatterns: []string{"git commit *"},
				Default:      core.AgentPermissionAllow,
			},
		}
		err := AuthorizeCommand(ctx, m, "agent", spec, CommandAuthorizationRequest{
			Command: []string{"git", "commit", "-m", "msg"},
			Source:  "git",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied by bash_permissions")
	})

	t.Run("bash ask requests unified approval action", func(t *testing.T) {
		hitl := &recordingApproveHITL{}
		perms := &core.PermissionSet{
			Executables: []core.ExecutablePermission{
				{Binary: "cargo"},
			},
		}
		m, err := NewPermissionManager("/ws", perms, nil, hitl)
		require.NoError(t, err)
		spec := &core.AgentRuntimeSpec{
			Bash: core.AgentBashPermissions{
				Default: core.AgentPermissionAsk,
			},
		}
		err = AuthorizeCommand(ctx, m, "agent", spec, CommandAuthorizationRequest{
			Command: []string{"cargo", "check"},
			Source:  "cli",
		})
		require.NoError(t, err)
		require.Equal(t, 1, hitl.calls)
		require.Len(t, hitl.requests, 1)
		assert.Equal(t, commandApprovalAction, hitl.requests[0].Permission.Action)
		assert.Equal(t, "cargo check", hitl.requests[0].Permission.Resource)
		assert.Equal(t, "cli", hitl.requests[0].Permission.Metadata["source"])
	})

	t.Run("bash ask without permission manager is rejected", func(t *testing.T) {
		spec := &core.AgentRuntimeSpec{
			Bash: core.AgentBashPermissions{
				Default: core.AgentPermissionAsk,
			},
		}
		err := AuthorizeCommand(ctx, nil, "agent", spec, CommandAuthorizationRequest{
			Command: []string{"cargo", "check"},
			Source:  "cli",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "approval required but permission manager missing")
	})
}

// ---- CheckNetwork — direction/port/host matching, default policies ----

func TestCheckNetwork(t *testing.T) {
	ctx := context.Background()

	declared := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/ws/**"},
		},
		Network: []core.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443},
			{Direction: "egress", Protocol: "tcp", Host: "**", Port: 80},
			{Direction: "ingress", Protocol: "tcp", Port: 8080},
			{Direction: "dns", Protocol: "udp"},
		},
	}

	t.Run("egress exact host+port match", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		require.NoError(t, m.CheckNetwork(ctx, "agent", "egress", "tcp", "api.example.com", 443))
	})

	t.Run("egress wildcard host matches any host on port 80", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		require.NoError(t, m.CheckNetwork(ctx, "agent", "egress", "tcp", "anything.com", 80))
	})

	t.Run("egress wrong port rejected", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		m.SetDefaultPolicy(core.AgentPermissionDeny)
		require.Error(t, m.CheckNetwork(ctx, "agent", "egress", "tcp", "api.example.com", 8443))
	})

	t.Run("ingress declared port matches", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		require.NoError(t, m.CheckNetwork(ctx, "agent", "ingress", "tcp", "", 8080))
	})

	t.Run("ingress undeclared port rejected", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		m.SetDefaultPolicy(core.AgentPermissionDeny)
		require.Error(t, m.CheckNetwork(ctx, "agent", "ingress", "tcp", "", 9090))
	})

	t.Run("dns direction matches declared dns rule", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		require.NoError(t, m.CheckNetwork(ctx, "agent", "dns", "udp", "", 0))
	})

	t.Run("undeclared host + Deny blocked", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		m.SetDefaultPolicy(core.AgentPermissionDeny)
		require.Error(t, m.CheckNetwork(ctx, "agent", "egress", "tcp", "unknown.com", 22))
	})

	t.Run("undeclared host + Allow passes", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		m.SetDefaultPolicy(core.AgentPermissionAllow)
		require.NoError(t, m.CheckNetwork(ctx, "agent", "egress", "tcp", "unknown.com", 22))
	})

	t.Run("undeclared host + Ask routes to HITL", func(t *testing.T) {
		hitl := &autoApproveHITL{}
		m, err := NewPermissionManager("/ws", declared, nil, hitl)
		require.NoError(t, err)
		m.SetDefaultPolicy(core.AgentPermissionAsk)
		require.NoError(t, m.CheckNetwork(ctx, "agent", "egress", "tcp", "unknown.com", 22))
		assert.Equal(t, 1, hitl.calls)
	})
}

// ---- CheckIPC ----

func TestCheckIPC(t *testing.T) {
	ctx := context.Background()

	declared := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/ws/**"},
		},
		IPC: []core.IPCPermission{
			{Kind: "socket", Target: "/tmp/agent.sock"},
			{Kind: "pipe", Target: "**"},
			{Kind: "signal", Target: "agent-proc", HITLRequired: true},
		},
	}

	t.Run("declared socket target passes", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		require.NoError(t, m.CheckIPC(ctx, "agent", "socket", "/tmp/agent.sock"))
	})

	t.Run("wildcard target matches any", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		require.NoError(t, m.CheckIPC(ctx, "agent", "pipe", "any-target"))
	})

	t.Run("undeclared target denied", func(t *testing.T) {
		m := newTestPM(t, "/ws", declared)
		err := m.CheckIPC(ctx, "agent", "socket", "/tmp/other.sock")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ipc scope missing")
	})

	t.Run("undeclared kind denied regardless of target", func(t *testing.T) {
		// "shm" is not in the declared IPC list at all
		m := newTestPM(t, "/ws", declared)
		err := m.CheckIPC(ctx, "agent", "shm", "some-segment")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ipc scope missing")
	})

	t.Run("declared kind with non-matching specific target denied", func(t *testing.T) {
		// "signal" is declared only for "agent-proc"; any other target should be denied
		m := newTestPM(t, "/ws", declared)
		err := m.CheckIPC(ctx, "agent", "signal", "other-proc")
		require.Error(t, err)
	})

	t.Run("HITLRequired triggers approval", func(t *testing.T) {
		hitl := &autoApproveHITL{}
		m, err := NewPermissionManager("/ws", declared, nil, hitl)
		require.NoError(t, err)
		require.NoError(t, m.CheckIPC(ctx, "agent", "signal", "agent-proc"))
		assert.Equal(t, 1, hitl.calls)
	})
}

// ---- inflateScopes — workspace placeholder expansion at construction time ----

func TestInflateScopes(t *testing.T) {
	ctx := context.Background()

	t.Run("${workspace} placeholder resolved in declared paths", func(t *testing.T) {
		declared := &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{
				{Action: core.FileSystemRead, Path: "${workspace}/src/**"},
			},
		}
		m, err := NewPermissionManager("/myproject", declared, nil, nil)
		require.NoError(t, err)
		m.SetDefaultPolicy(core.AgentPermissionDeny)

		// placeholder resolved → /myproject/src/** — file within should be accessible
		require.NoError(t, m.CheckFileAccess(ctx, "agent", core.FileSystemRead, "/myproject/src/main.go"))

		// file outside the inflated path should be denied
		require.Error(t, m.CheckFileAccess(ctx, "agent", core.FileSystemRead, "/myproject/other/file.go"))
	})

	t.Run("{{workspace}} variant also resolved", func(t *testing.T) {
		declared := &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{
				{Action: core.FileSystemRead, Path: "{{workspace}}/lib/**"},
			},
		}
		m, err := NewPermissionManager("/proj", declared, nil, nil)
		require.NoError(t, err)
		m.SetDefaultPolicy(core.AgentPermissionDeny)

		require.NoError(t, m.CheckFileAccess(ctx, "agent", core.FileSystemRead, "/proj/lib/util.go"))
		require.Error(t, m.CheckFileAccess(ctx, "agent", core.FileSystemRead, "/proj/src/main.go"))
	})
}
