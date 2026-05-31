package main

import (
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

func TestAuditCleanRepoRoot(t *testing.T) {
	root := repoRoot(t)
	findings := audit(root)
	if len(findings) > 0 {
		var msgs []string
		for _, f := range findings {
			msgs = append(msgs, f.File+":"+itoa(f.Line)+" "+f.Message)
		}
		t.Fatalf("expected no findings in repo root, got %d:\n%s", len(findings), strings.Join(msgs, "\n"))
	}
}

func TestAuditCatchesEnvViolation(t *testing.T) {
	workspace := writeMinimalWorkspace(t)

	violationDir := filepath.Join(workspace, "app", "relurpish")
	testhelper.MustMkdirAll(t, violationDir)
	testhelper.MustWrite(t, filepath.Join(violationDir, "violation.go"), `package relurpish
import "os"
func get() string {
	return os.Getenv("SECRET_KEY")
}
`)

	findings := audit(workspace)
	if len(findings) == 0 {
		t.Fatal("expected env boundary violation, got none")
	}
	found := false
	for _, f := range findings {
		if strings.Contains(f.Message, "environment boundary violation") &&
			strings.Contains(f.Message, "os.Getenv") &&
			strings.Contains(f.File, "app/relurpish/violation.go") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected env violation in app/relurpish/violation.go with os.Getenv, got: %+v", findings)
	}
}

func TestAuditCatchesConfigPathViolation(t *testing.T) {
	workspace := writeMinimalWorkspace(t)

	violationDir := filepath.Join(workspace, "app", "relurpish")
	testhelper.MustMkdirAll(t, violationDir)
	testhelper.MustWrite(t, filepath.Join(violationDir, "violation.go"), `package relurpish
import "os"
func read() {
	_, _ = os.ReadFile("relurpify_cfg/workspace.yaml")
}
`)

	findings := audit(workspace)
	if len(findings) == 0 {
		t.Fatal("expected config path boundary violation, got none")
	}
	found := false
	for _, f := range findings {
		if strings.Contains(f.Message, "config path boundary violation") &&
			strings.Contains(f.Message, "os.ReadFile") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected config path violation with os.ReadFile, got: %+v", findings)
	}
}

func TestAuditFlagsLookupEnv(t *testing.T) {
	workspace := writeMinimalWorkspace(t)

	violationDir := filepath.Join(workspace, "app", "relurpish")
	testhelper.MustMkdirAll(t, violationDir)
	testhelper.MustWrite(t, filepath.Join(violationDir, "violation.go"), `package relurpish
import "os"
func lookup() string {
	v, _ := os.LookupEnv("SECRET")
	return v
}
`)

	findings := audit(workspace)
	if len(findings) == 0 {
		t.Fatal("expected env boundary violation for LookupEnv, got none")
	}
	found := false
	for _, f := range findings {
		if strings.Contains(f.Message, "environment boundary violation") &&
			strings.Contains(f.Message, "os.LookupEnv") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected LookupEnv violation, got: %+v", findings)
	}
}

func TestAuditFlagsSetenv(t *testing.T) {
	workspace := writeMinimalWorkspace(t)

	violationDir := filepath.Join(workspace, "app", "relurpish")
	testhelper.MustMkdirAll(t, violationDir)
	testhelper.MustWrite(t, filepath.Join(violationDir, "violation.go"), `package relurpish
import "os"
func set() {
	_ = os.Setenv("SECRET", "value")
}
`)

	findings := audit(workspace)
	if len(findings) == 0 {
		t.Fatal("expected env boundary violation for Setenv, got none")
	}
	found := false
	for _, f := range findings {
		if strings.Contains(f.Message, "environment boundary violation") &&
			strings.Contains(f.Message, "os.Setenv") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Setenv violation, got: %+v", findings)
	}
}

func TestAuditFlagsConstBuiltConfigPath(t *testing.T) {
	workspace := writeMinimalWorkspace(t)

	violationDir := filepath.Join(workspace, "app", "relurpish")
	testhelper.MustMkdirAll(t, violationDir)
	testhelper.MustWrite(t, filepath.Join(violationDir, "violation.go"), `package relurpish
import (
	"os"
	"path/filepath"
)
func read() {
	_, _ = os.ReadFile(filepath.Join("/workspace", "relurpify_cfg", "workspace.yaml"))
}
`)

	findings := audit(workspace)
	if len(findings) == 0 {
		t.Fatal("expected config path boundary violation for const-built path, got none")
	}
	found := false
	for _, f := range findings {
		if strings.Contains(f.Message, "config path boundary violation") &&
			strings.Contains(f.Message, "os.ReadFile") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected config path violation with const-built path, got: %+v", findings)
	}
}

func TestAuditTestsuiteExemptionIsPrefixNotSubstring(t *testing.T) {
	workspace := writeMinimalWorkspace(t)

	dir := filepath.Join(workspace, "app", "relurpish")
	testhelper.MustMkdirAll(t, dir)
	testhelper.MustWrite(t, filepath.Join(dir, "violation.go"), `package relurpish
import "os"
func get() string {
	return os.Getenv("SECRET_KEY")
}
`)

	findings := audit(workspace)
	if len(findings) == 0 {
		t.Fatal("expected env violation outside testsuite directory, got none")
	}
}

func TestAuditFrameworkCfgloadExempt(t *testing.T) {
	workspace := writeMinimalWorkspace(t)

	filePath := filepath.Join(workspace, "framework", "cfgload", "loader.go")
	testhelper.MustWrite(t, filePath, `package cfgload
import "os"
func get() string {
	return os.Getenv("CONFIG_KEY")
}
`)

	findings := audit(workspace)
	for _, f := range findings {
		if f.File == "framework/cfgload/loader.go" {
			t.Fatalf("expected no finding in framework/cfgload/, got: %+v", f)
		}
	}
}

func TestAuditTestsuiteExempt(t *testing.T) {
	workspace := writeMinimalWorkspace(t)

	dir := filepath.Join(workspace, "testsuite", "somepkg")
	testhelper.MustMkdirAll(t, dir)
	testhelper.MustWrite(t, filepath.Join(dir, "helper.go"), `package somepkg
import "os"
func get() string {
	return os.Getenv("TEST_KEY")
}
`)

	findings := audit(workspace)
	for _, f := range findings {
		if strings.Contains(f.File, "testsuite/") {
			t.Fatalf("expected no finding in testsuite/, got: %+v", f)
		}
	}
}

func TestAuditSkipsTestdata(t *testing.T) {
	workspace := writeMinimalWorkspace(t)

	dir := filepath.Join(workspace, "testdata")
	testhelper.MustMkdirAll(t, dir)
	testhelper.MustWrite(t, filepath.Join(dir, "helper.go"), `package testdata
import "os"
func get() string {
	return os.Getenv("SECRET")
}
`)

	findings := audit(workspace)
	for _, f := range findings {
		if strings.Contains(f.File, "testdata/") {
			t.Fatalf("expected no finding in testdata/, got: %+v", f)
		}
	}
}

func TestAuditSkipsVendor(t *testing.T) {
	workspace := writeMinimalWorkspace(t)

	dir := filepath.Join(workspace, "vendor", "somepkg")
	testhelper.MustMkdirAll(t, dir)
	testhelper.MustWrite(t, filepath.Join(dir, "helper.go"), `package somepkg
import "os"
func get() string {
	return os.Getenv("SECRET")
}
`)

	findings := audit(workspace)
	for _, f := range findings {
		if strings.Contains(f.File, "vendor/") {
			t.Fatalf("expected no finding in vendor/, got: %+v", f)
		}
	}
}

func TestAuditNoRepoMarkerReturnsEmpty(t *testing.T) {
	workspace := t.TempDir()
	dir := filepath.Join(workspace, "app", "relurpish")
	testhelper.MustMkdirAll(t, dir)
	testhelper.MustWrite(t, filepath.Join(dir, "violation.go"), `package relurpish
import "os"
func get() string {
	return os.Getenv("SECRET_KEY")
}
`)

	findings := audit(workspace)
	if len(findings) != 0 {
		t.Fatalf("expected no findings without framework/cfgload marker, got: %+v", findings)
	}
}

func writeMinimalWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	testhelper.WriteValidWorkspace(t, workspace)
	testhelper.MustMkdirAll(t, filepath.Join(workspace, "framework", "cfgload"))
	return workspace
}

func repoRoot(t *testing.T) string {
	t.Helper()
	return testhelper.RepoRoot(t)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
