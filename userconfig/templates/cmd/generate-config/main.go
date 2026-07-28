// Command generate-config writes the embedded workspace template tree to an
// output directory, mirroring its layout 1:1. It is the producer of the
// checked-in relurpify_cfg/ tree (see `make generate-config`).
package main

import (
	"fmt"
	"os"

	"codeburg.org/lexbit/relurpify/userconfig/templates"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: generate-config <output-dir>")
		os.Exit(1)
	}
	if err := templates.GenerateConfig(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "generate-config: %v\n", err)
		os.Exit(1)
	}
}
