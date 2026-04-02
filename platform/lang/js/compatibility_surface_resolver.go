package js

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

func (r *CompatibilitySurfaceResolver) BackendID() string { return "js" }

func (r *CompatibilitySurfaceResolver) Supports(req agentenv.CompatibilitySurfaceRequest) bool {
	for _, file := range req.Files {
		if hasJSExtension(file) {
			return true
		}
	}
	for _, file := range req.FileContents {
		if hasJSExtension(fmt.Sprint(file["path"])) {
			return true
		}
	}
	return false
}

func (r *CompatibilitySurfaceResolver) ExtractSurface(_ context.Context, req agentenv.CompatibilitySurfaceRequest) (agentenv.CompatibilitySurface, bool, error) {
	exportFuncRe := regexp.MustCompile(`^export\s+function\s+([A-Za-z_]\w*)\s*\(`)
	exportClassRe := regexp.MustCompile(`^export\s+class\s+([A-Za-z_]\w*)`)
	exportNamedRe := regexp.MustCompile(`^export\s+\{\s*([A-Za-z_]\w*)`)
	surface := agentenv.CompatibilitySurface{Metadata: map[string]any{"language": "js", "source": "platform.lang.js"}}
	for _, file := range req.FileContents {
		path := strings.TrimSpace(fmt.Sprint(file["path"]))
		content := strings.TrimSpace(fmt.Sprint(file["content"]))
		for idx, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if match := exportFuncRe.FindStringSubmatch(trimmed); len(match) > 0 {
				surface.Functions = append(surface.Functions, map[string]any{"name": match[1], "signature": trimmed, "location": fmt.Sprintf("%s:%d", path, idx+1)})
			}
			if match := exportClassRe.FindStringSubmatch(trimmed); len(match) > 0 {
				surface.Types = append(surface.Types, map[string]any{"name": match[1], "location": fmt.Sprintf("%s:%d", path, idx+1)})
			}
			if match := exportNamedRe.FindStringSubmatch(trimmed); len(match) > 0 {
				surface.Functions = append(surface.Functions, map[string]any{"name": match[1], "signature": trimmed, "location": fmt.Sprintf("%s:%d", path, idx+1)})
			}
		}
	}
	return surface, len(surface.Functions) > 0 || len(surface.Types) > 0, nil
}

func hasJSExtension(path string) bool {
	path = strings.TrimSpace(path)
	return strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".jsx") || strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")
}
