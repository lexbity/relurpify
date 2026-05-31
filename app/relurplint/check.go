package main

import (
	"fmt"
	"strings"
)

type Check interface {
	Name() string
	Run(workspace string) []Diagnostic
}

var registry = make(map[string]Check)

func registerCheck(c Check) {
	registry[c.Name()] = c
}

func selected(flag string) ([]Check, error) {
	if flag == "all" {
		result := make([]Check, 0, len(registry))
		for _, c := range registry {
			result = append(result, c)
		}
		return result, nil
	}

	names := strings.Split(flag, ",")
	var result []Check
	for _, name := range names {
		name = strings.TrimSpace(name)
		c, ok := registry[name]
		if !ok {
			return nil, fmt.Errorf("unknown check %q; available: %s", name, availableChecks())
		}
		result = append(result, c)
	}
	return result, nil
}

func availableChecks() string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	if len(names) == 0 {
		return "(none registered)"
	}
	return strings.Join(names, ", ")
}
