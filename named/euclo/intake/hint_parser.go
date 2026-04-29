package intake

import (
	"regexp"
	"strings"
)

// HintParser extracts structured hints from user messages.
type HintParser struct {
	// patterns for hint detection
	contextHintPattern  *regexp.Regexp
	sessionHintPattern  *regexp.Regexp
	followUpHintPattern *regexp.Regexp
	agentModePattern    *regexp.Regexp
	workspaceScopePattern *regexp.Regexp
	filePathPattern     *regexp.Regexp
	ingestPolicyPattern *regexp.Regexp
	incrementalPattern  *regexp.Regexp
}

// NewHintParser creates a new hint parser with compiled regex patterns.
func NewHintParser() *HintParser {
	return &HintParser{
		// Match patterns like "context-hint: value" or "@context-hint: value"
		contextHintPattern:  regexp.MustCompile(`(?i)(?:@?context-hint[:\s]+)([^\n]+)`),
		sessionHintPattern:  regexp.MustCompile(`(?i)(?:@?session-hint[:\s]+)([^\n]+)`),
		followUpHintPattern: regexp.MustCompile(`(?i)(?:@?follow-up[:\s]+)([^\n]+)`),
		agentModePattern:    regexp.MustCompile(`(?i)(?:@?mode[:\s]+)([^\n]+)`),
		workspaceScopePattern: regexp.MustCompile(`(?i)(?:@?workspace-scope[:\s]+)([^\n]+)`),
		// Match file paths (simplified - matches common patterns)
		filePathPattern: regexp.MustCompile(`(?i)(?:file[:\s]+|path[:\s]+)?([a-zA-Z0-9_\-\./]+\.[a-zA-Z0-9]+)`),
		// Match ingest policy directives
		ingestPolicyPattern: regexp.MustCompile(`(?i)(?:@?ingest[:\s]+)(files_only|incremental|full)`),
		// Match incremental since directive
		incrementalPattern: regexp.MustCompile(`(?i)(?:@?since[:\s]+)([a-f0-9]{7,40})`),
	}
}

// ParseResult holds the extracted hints from a message.
type ParseResult struct {
	ContextHint     string
	SessionHint     string
	FollowUpHint    string
	AgentModeHint   string
	WorkspaceScopes []string
	ExplicitFiles   []string
	IngestPolicy    string
	IncrementalSince string
}

// Parse extracts all hints from a user message.
func (p *HintParser) Parse(message string) *ParseResult {
	result := &ParseResult{}

	// Extract context hint
	if matches := p.contextHintPattern.FindStringSubmatch(message); len(matches) > 1 {
		result.ContextHint = strings.TrimSpace(matches[1])
	}

	// Extract session hint
	if matches := p.sessionHintPattern.FindStringSubmatch(message); len(matches) > 1 {
		result.SessionHint = strings.TrimSpace(matches[1])
	}

	// Extract follow-up hint
	if matches := p.followUpHintPattern.FindStringSubmatch(message); len(matches) > 1 {
		result.FollowUpHint = strings.TrimSpace(matches[1])
	}

	// Extract agent mode hint
	if matches := p.agentModePattern.FindStringSubmatch(message); len(matches) > 1 {
		result.AgentModeHint = strings.TrimSpace(matches[1])
	}

	// Extract workspace scopes
	if matches := p.workspaceScopePattern.FindStringSubmatch(message); len(matches) > 1 {
		scopes := strings.Split(matches[1], ",")
		for _, scope := range scopes {
			s := strings.TrimSpace(scope)
			if s != "" {
				result.WorkspaceScopes = append(result.WorkspaceScopes, s)
			}
		}
	}

	// Extract file paths
	fileMatches := p.filePathPattern.FindAllStringSubmatch(message, -1)
	seenFiles := make(map[string]bool)
	for _, match := range fileMatches {
		if len(match) > 1 {
			file := strings.TrimSpace(match[1])
			if file != "" && !seenFiles[file] {
				result.ExplicitFiles = append(result.ExplicitFiles, file)
				seenFiles[file] = true
			}
		}
	}

	// Extract ingest policy
	if matches := p.ingestPolicyPattern.FindStringSubmatch(message); len(matches) > 1 {
		result.IngestPolicy = strings.ToLower(strings.TrimSpace(matches[1]))
	}

	// Extract incremental since ref
	if matches := p.incrementalPattern.FindStringSubmatch(message); len(matches) > 1 {
		result.IncrementalSince = strings.TrimSpace(matches[1])
	}

	return result
}

// StripHints removes all hint directives from the message, returning the clean instruction.
func (p *HintParser) StripHints(message string) string {
	// Remove all hint patterns
	patterns := []*regexp.Regexp{
		p.contextHintPattern,
		p.sessionHintPattern,
		p.followUpHintPattern,
		p.agentModePattern,
		p.workspaceScopePattern,
		p.ingestPolicyPattern,
		p.incrementalPattern,
	}

	clean := message
	for _, pattern := range patterns {
		clean = pattern.ReplaceAllString(clean, "")
	}

	// Clean up extra whitespace
	clean = regexp.MustCompile(`\s+`).ReplaceAllString(clean, " ")
	clean = strings.TrimSpace(clean)

	return clean
}

// HasExplicitFiles checks if the message contains explicit file path references.
func (p *HintParser) HasExplicitFiles(message string) bool {
	return len(p.filePathPattern.FindAllString(message, -1)) > 0
}

// DetectIngestPolicy determines the appropriate ingest policy based on message content.
func (p *HintParser) DetectIngestPolicy(message string, hasExplicitFiles bool) string {
	// Check for explicit policy directive
	if matches := p.ingestPolicyPattern.FindStringSubmatch(message); len(matches) > 1 {
		return strings.ToLower(strings.TrimSpace(matches[1]))
	}

	// Infer from content
	if hasExplicitFiles {
		return "files_only"
	}

	if p.incrementalPattern.MatchString(message) {
		return "incremental"
	}

	// Default to full ingestion
	return "full"
}
