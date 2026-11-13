// Command relurpishcheck enforces the frontend's production source boundary.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/tooling/arch"
)

func main() {
	count := 0
	err := filepath.WalkDir("app/relurpish", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		violations, err := arch.CheckRelurpishV2Source(filepath.ToSlash(path), source)
		if err != nil {
			return err
		}
		for _, violation := range violations {
			fmt.Println(violation)
			count++
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if count != 0 {
		os.Exit(1)
	}
	listing, err := exec.Command("go", "test", "./app/relurpish/tui", "./app/relurpish/euclotui", "./app/relurpish/demo", "./app/relurpish/testsuite", "-list", "^TestV2").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot enumerate required tests:", err)
		os.Exit(1)
	}
	if missing := arch.MissingRelurpishV2Tests(string(listing)); len(missing) != 0 {
		fmt.Fprintln(os.Stderr, "missing required tests:", missing)
		os.Exit(1)
	}
	integration, err := exec.Command("go", "test", "-tags=relurpish_v2_integration", "./app/relurpish/testsuite", "-list", "^TestV2").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot enumerate integration tests:", err)
		os.Exit(1)
	}
	if missing := arch.MissingRelurpishV2IntegrationTests(string(integration)); len(missing) != 0 {
		fmt.Fprintln(os.Stderr, "missing integration tests:", missing)
		os.Exit(1)
	}
	fmt.Println("[PASS] Relurpish frontend source boundary")
}
