package agenttest

import (
	"log"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func shouldResetOllama(err error, patterns []string) bool {
	if err == nil || len(patterns) == 0 {
		return false
	}
	msg := err.Error()
	for _, raw := range patterns {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		re, reErr := regexp.Compile(raw)
		if reErr != nil {
			continue
		}
		if re.MatchString(msg) {
			return true
		}
	}
	return false
}

func maybeResetOllama(logger *log.Logger, opts RunOptions, modelName string) {
	mode := strings.ToLower(strings.TrimSpace(opts.OllamaReset))
	if mode == "" || mode == "none" {
		return
	}
	bin := strings.TrimSpace(opts.OllamaBinary)
	if bin == "" {
		bin = "ollama"
	}
	service := strings.TrimSpace(opts.OllamaService)
	if service == "" {
		service = "ollama"
	}

	switch mode {
	case "model":
		if strings.TrimSpace(modelName) == "" {
			return
		}
		// Best-effort unload of the model so the next call re-loads it fresh.
		_ = runBestEffort(logger, bin, "stop", modelName)
		time.Sleep(200 * time.Millisecond)
	case "server":
		// Best-effort restart: prefer systemctl if present.
		if err := runBestEffort(logger, "systemctl", "restart", service); err != nil {
			// Fallback: try to stop running models.
			if strings.TrimSpace(modelName) != "" {
				_ = runBestEffort(logger, bin, "stop", modelName)
			}
		}
		time.Sleep(500 * time.Millisecond)
	default:
		return
	}
}

func runBestEffort(logger *log.Logger, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if logger != nil {
		if len(out) > 0 {
			logger.Printf("ollama reset cmd=%s args=%v out=%s", name, args, strings.TrimSpace(string(out)))
		} else {
			logger.Printf("ollama reset cmd=%s args=%v", name, args)
		}
		if err != nil {
			logger.Printf("ollama reset error: %v", err)
		}
	}
	return err
}
