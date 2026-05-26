package main

import (
	"flag"
	"fmt"
	"os"

	"codeburg.org/lexbit/relurpify/framework/templates"
)

func main() {
	var output string
	flag.StringVar(&output, "output", "", "Output directory for generated templates")
	flag.Parse()
	if output == "" {
		fmt.Fprintln(os.Stderr, "--output is required")
		os.Exit(1)
	}
	if err := templates.GenerateWorkspaceTemplates(output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
