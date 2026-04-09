package agenttest

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var ollamaHTTPClient = &http.Client{Timeout: 5 * time.Second}

type BackendModelProvenance struct {
	RequestedModel string         `json:"requested_model"`
	LoadedName     string         `json:"loaded_name,omitempty"`
	LoadedModel    string         `json:"loaded_model,omitempty"`
	Digest         string         `json:"digest,omitempty"`
	Details        map[string]any `json:"details,omitempty"`
}

func shouldResetBackend(err error, patterns []string) bool {
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

func maybeResetBackend(logger *log.Logger, opts RunOptions, modelName string) {
	mode := strings.ToLower(strings.TrimSpace(opts.BackendReset))
	if mode == "" || mode == "none" {
		return
	}
	bin := strings.TrimSpace(opts.BackendBinary)
	if bin == "" {
		bin = "ollama"
	}
	service := strings.TrimSpace(opts.BackendService)
	if service == "" {
		service = "ollama"
	}

	switch mode {
	case "model":
		if strings.TrimSpace(modelName) == "" {
			return
		}
		_ = runBestEffort(logger, bin, "stop", modelName)
		time.Sleep(200 * time.Millisecond)
	case "server":
		if err := runBestEffort(logger, "systemctl", "restart", service); err != nil {
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
			logger.Printf("backend reset cmd=%s args=%v out=%s", name, args, strings.TrimSpace(string(out)))
		} else {
			logger.Printf("backend reset cmd=%s args=%v", name, args)
		}
		if err != nil {
			logger.Printf("backend reset error: %v", err)
		}
	}
	return err
}

func shouldPreflightBackend(recordingMode string) bool {
	mode := strings.ToLower(strings.TrimSpace(recordingMode))
	return mode == "" || mode == "off" || mode == "record"
}

func preflightBackend(endpoint, model string) error {
	provenance, err := lookupBackendModelProvenance(endpoint, model)
	if err != nil {
		return err
	}
	if provenance == nil {
		return fmt.Errorf("backend model %q is not loaded at %s", strings.TrimSpace(model), strings.TrimRight(strings.TrimSpace(endpoint), "/"))
	}
	return nil
}

func lookupBackendModelProvenance(endpoint, model string) (*BackendModelProvenance, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	model = strings.TrimSpace(model)
	if endpoint == "" || model == "" {
		return nil, nil
	}

	tagsReq, err := http.NewRequest(http.MethodGet, endpoint+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("backend preflight request: %w", err)
	}
	tagsResp, err := ollamaHTTPClient.Do(tagsReq)
	if err != nil {
		return nil, fmt.Errorf("backend preflight tags failed for %s: %w", endpoint, err)
	}
	defer tagsResp.Body.Close()
	if tagsResp.StatusCode < 200 || tagsResp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(tagsResp.Body, 2048))
		return nil, fmt.Errorf("backend preflight tags failed for %s: status %d: %s", endpoint, tagsResp.StatusCode, strings.TrimSpace(string(body)))
	}

	psReq, err := http.NewRequest(http.MethodGet, endpoint+"/api/ps", nil)
	if err != nil {
		return nil, fmt.Errorf("backend preflight request: %w", err)
	}
	psResp, err := ollamaHTTPClient.Do(psReq)
	if err != nil {
		return nil, fmt.Errorf("backend preflight ps failed for %s: %w", endpoint, err)
	}
	defer psResp.Body.Close()
	if psResp.StatusCode < 200 || psResp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(psResp.Body, 2048))
		return nil, fmt.Errorf("backend preflight ps failed for %s: status %d: %s", endpoint, psResp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Models []struct {
			Name    string         `json:"name"`
			Model   string         `json:"model"`
			Digest  string         `json:"digest"`
			Details map[string]any `json:"details"`
		} `json:"models"`
	}
	if err := json.NewDecoder(psResp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("backend preflight ps decode failed for %s: %w", endpoint, err)
	}
	for _, item := range payload.Models {
		if strings.EqualFold(strings.TrimSpace(item.Name), model) || strings.EqualFold(strings.TrimSpace(item.Model), model) {
			return &BackendModelProvenance{
				RequestedModel: model,
				LoadedName:     strings.TrimSpace(item.Name),
				LoadedModel:    strings.TrimSpace(item.Model),
				Digest:         strings.TrimSpace(item.Digest),
				Details:        item.Details,
			}, nil
		}
	}
	return nil, nil
}
