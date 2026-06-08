package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/tooling/arch"
	"gopkg.in/yaml.v3"
)

type ExceptionsFile struct {
	Version    int                       `yaml:"version"`
	Directions []arch.DirectionException `yaml:"direction_violations"`
}

func main() {
	mode := flag.String("mode", "warn", "enforcement mode: warn or enforce")
	check := flag.String("check", "all", "check to run: all, direction, cycles, nobucket")
	flag.Parse()

	root, _ := os.Getwd()
	pkgs, err := arch.ListPackages(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listing packages: %v\n", err)
		os.Exit(1)
	}

	forward, reverse := arch.ImportGraph(pkgs)

	exceptionsPath := filepath.Join(root, "tooling", "arch", "exceptions.yaml")
	domainExceptions := loadExceptions(exceptionsPath)

	exitResults := []arch.Result{}
	hadFailure := false

	if *check == "all" || *check == "direction" {
		vios := arch.CheckDomainDirection(pkgs, forward, *mode, domainExceptions)
		fmt.Print(arch.Report("domain-direction", vios))
		if len(vios) > 0 {
			hadFailure = true
		}
		exitResults = append(exitResults, arch.Result{Name: "domain-direction", Violations: vios})
	}

	if *check == "all" || *check == "cycles" {
		cycles := arch.DomainCycleReport(pkgs, forward)
		fmt.Print(arch.Report("domain-cycles", cycles))
	}

	if *check == "all" || *check == "nobucket" {
		vios := arch.CheckNoBucket(pkgs, reverse, root)
		fmt.Print(arch.Report("no-bucket", vios))
		if len(vios) > 0 {
			hadFailure = true
		}
		exitResults = append(exitResults, arch.Result{Name: "no-bucket", Violations: vios})
	}

	if !hadFailure {
		fmt.Println("[PASS] All domain checks passed")
	}

	if *mode == "warn" {
		os.Exit(0)
	}
	os.Exit(arch.ExitCode(exitResults))
}

func loadExceptions(path string) map[string]map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		fmt.Fprintf(os.Stderr, "error reading exceptions: %v\n", err)
		os.Exit(1)
	}
	var f ExceptionsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing exceptions: %v\n", err)
		os.Exit(1)
	}
	out := make(map[string]map[string]bool)
	for _, e := range f.Directions {
		if out[e.SrcDomain] == nil {
			out[e.SrcDomain] = make(map[string]bool)
		}
		out[e.SrcDomain][e.DstDomain] = true
	}
	return out
}
