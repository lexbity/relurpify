package main

import (
	"fmt"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/tooling/arch"
)

func main() {
	root, _ := os.Getwd()
	allowlistPath := filepath.Join(root, "devdocs", "ref", "arch-allowlist.yaml")

	results, err := arch.RunAll(root, allowlistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	arch.PrintResults(results)
	os.Exit(arch.ExitCode(results))
}
