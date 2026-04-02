package golang

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
)

type CompatibilitySurfaceResolver struct{}

func NewCompatibilitySurfaceResolver() *CompatibilitySurfaceResolver {
	return &CompatibilitySurfaceResolver{}
}

func (r *CompatibilitySurfaceResolver) BackendID() string { return "go" }

func (r *CompatibilitySurfaceResolver) Supports(req agentenv.CompatibilitySurfaceRequest) bool {
	for _, file := range req.Files {
		if strings.HasSuffix(strings.TrimSpace(file), ".go") {
			return true
		}
	}
	for _, file := range req.FileContents {
		if strings.HasSuffix(strings.TrimSpace(fmt.Sprint(file["path"])), ".go") {
			return true
		}
	}
	return false
}

func (r *CompatibilitySurfaceResolver) ExtractSurface(_ context.Context, req agentenv.CompatibilitySurfaceRequest) (agentenv.CompatibilitySurface, bool, error) {
	funcRe := regexp.MustCompile(`^func\s+([A-Z]\w*)\s*\(([^)]*)\)`)
	typeRe := regexp.MustCompile(`^type\s+([A-Z]\w*)\s+`)
	surface := agentenv.CompatibilitySurface{Metadata: map[string]any{"language": "go", "source": "platform.lang.go"}}
	for _, file := range req.FileContents {
		path := strings.TrimSpace(fmt.Sprint(file["path"]))
		content := strings.TrimSpace(fmt.Sprint(file["content"]))
		for idx, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if match := funcRe.FindStringSubmatch(trimmed); len(match) > 0 {
				surface.Functions = append(surface.Functions, map[string]any{"name": match[1], "signature": trimmed, "location": fmt.Sprintf("%s:%d", path, idx+1)})
			}
			if match := typeRe.FindStringSubmatch(trimmed); len(match) > 0 {
				surface.Types = append(surface.Types, map[string]any{"name": match[1], "location": fmt.Sprintf("%s:%d", path, idx+1)})
			}
		}
	}
	return surface, len(surface.Functions) > 0 || len(surface.Types) > 0, nil
}
