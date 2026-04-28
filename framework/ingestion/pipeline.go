package ingestion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

// AcquireFromFile creates a pipeline for ingesting a file.
func AcquireFromFile(ctx context.Context, path string, principal identity.SubjectRef, policy *contextpolicy.ContextPolicyBundle, store *knowledge.ChunkStore) (*Pipeline, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	raw := RawIngestion{
		Content:           content,
		SourcePrincipal:   principal,
		AcquisitionMethod: "file_read",
		AcquiredAt:        time.Now().UTC(),
		FilePath:          path,
		MIMEHint:          detectMIMEType(path, content),
	}

	return newPipeline(raw, policy, store), nil
}

// AcquireFromToolOutput creates a pipeline for ingesting tool output.
func AcquireFromToolOutput(ctx context.Context, output []byte, principal identity.SubjectRef, policy *contextpolicy.ContextPolicyBundle, store *knowledge.ChunkStore) (*Pipeline, error) {
	raw := RawIngestion{
		Content:           output,
		SourcePrincipal:   principal,
		AcquisitionMethod: "tool_output",
		AcquiredAt:        time.Now().UTC(),
		MIMEHint:          detectMIMEType("", output),
	}

	return newPipeline(raw, policy, store), nil
}

// AcquireFromUserInput creates a pipeline for ingesting user input.
func AcquireFromUserInput(ctx context.Context, input string, principal identity.SubjectRef, policy *contextpolicy.ContextPolicyBundle, store *knowledge.ChunkStore) (*Pipeline, error) {
	raw := RawIngestion{
		Content:           []byte(input),
		SourcePrincipal:   principal,
		AcquisitionMethod: "user_input",
		AcquiredAt:        time.Now().UTC(),
		MIMEHint:          "text/plain",
	}

	return newPipeline(raw, policy, store), nil
}

// AcquireFromCapabilityResult creates a pipeline for ingesting capability results.
func AcquireFromCapabilityResult(ctx context.Context, result []byte, contentType string, principal identity.SubjectRef, policy *contextpolicy.ContextPolicyBundle, store *knowledge.ChunkStore) (*Pipeline, error) {
	raw := RawIngestion{
		Content:           result,
		SourcePrincipal:   principal,
		AcquisitionMethod: "capability_result",
		AcquiredAt:        time.Now().UTC(),
		MIMEHint:          contentType,
	}

	return newPipeline(raw, policy, store), nil
}

func newPipeline(raw RawIngestion, policy *contextpolicy.ContextPolicyBundle, store *knowledge.ChunkStore) *Pipeline {
	return &Pipeline{
		raw:           raw,
		policy:        policy,
		evaluator:     contextpolicy.NewEvaluator(policy),
		store:         store,
		scanners:      defaultScanners(),
		quarantineDir: "relurpify_cfg/quarantine",
	}
}

// SetQuarantineDir sets the quarantine directory path.
func (p *Pipeline) SetQuarantineDir(dir string) {
	p.quarantineDir = dir
}

// SetScanners sets custom scanners for the pipeline.
func (p *Pipeline) SetScanners(scanners []Scanner) {
	p.scanners = scanners
}

// Run executes the six stages of the ingestion pipeline.
func (p *Pipeline) Run(ctx context.Context) (*IngestResult, error) {
	result := &IngestResult{
		Disposition: DispositionReject, // default until proven otherwise
	}

	// Stage 1: Acquisition (already done via raw ingestion)

	// Stage 2: Parsing + Typing
	typed, err := p.stage2Parse(ctx)
	if err != nil {
		if isBinaryContent(p.raw.Content) {
			result.Disposition = DispositionReject
			result.Reason = DispositionReason{Stage: "parsing", Explanation: "binary content rejected"}
			// Emit event for binary rejection
			return result, nil
		}
		result.Error = fmt.Errorf("stage 2 parsing: %w", err)
		return result, result.Error
	}

	// Stage 3: Scanning
	scanReports := p.stage3Scan(ctx, typed)

	// Stage 4: Enrichment
	edges := p.stage4Enrich(ctx, typed)

	// Stage 5: Admission
	disposition, reason := p.stage5Admission(ctx, typed, scanReports)
	result.Disposition = disposition
	result.Reason = reason

	if disposition == DispositionReject {
		result.ChunksRejected = len(typed.ChunkBoundaries)
		return result, nil
	}

	// Stage 6: Commit
	commitResult, err := p.stage6Commit(ctx, typed, edges, disposition)
	if err != nil {
		result.Error = fmt.Errorf("stage 6 commit: %w", err)
		return result, result.Error
	}

	result.ChunksCommitted = commitResult.committed
	result.ChunksQuarantined = commitResult.quarantined
	result.ChunkIDs = commitResult.chunkIDs
	result.EdgesCreated = commitResult.edgesCreated
	result.EventsEmitted = commitResult.eventsEmitted
	result.QuarantinePath = commitResult.quarantinePath

	return result, nil
}

// stage2Parse performs parsing and typing.
func (p *Pipeline) stage2Parse(ctx context.Context) (*TypedIngestion, error) {
	parser := selectParser(p.raw.MIMEHint, p.raw.FilePath)
	if parser == nil {
		return nil, fmt.Errorf("no parser for mime type %s", p.raw.MIMEHint)
	}

	return parser.Parse(ctx, p.raw)
}

// stage3Scan performs scanning on all chunks.
func (p *Pipeline) stage3Scan(ctx context.Context, typed *TypedIngestion) []ScannerReport {
	var reports []ScannerReport

	for _, boundary := range typed.ChunkBoundaries {
		chunk := TypedChunk{
			Content:     typed.Content[boundary.Start:boundary.End],
			Boundary:    boundary,
			ContentType: typed.ContentType,
			Metadata:    typed.Metadata,
		}

		report := ScannerReport{
			Details: make(map[string]ScanResult),
		}

		for _, scanner := range p.scanners {
			scanResult := scanner.Scan(ctx, chunk)
			report.Details[scanner.Name()] = scanResult

			// Aggregate suspicion scores (take max)
			if scanResult.SuspicionScore > report.SuspicionScore {
				report.SuspicionScore = scanResult.SuspicionScore
			}

			// Aggregate flags
			report.Flags = append(report.Flags, scanResult.Flags...)
		}

		reports = append(reports, report)
	}

	return reports
}

// stage4Enrich extracts edges and relationships.
func (p *Pipeline) stage4Enrich(ctx context.Context, typed *TypedIngestion) []CandidateEdges {
	var edges []CandidateEdges

	// Simple enrichment - in real implementation would parse imports, calls, etc.
	for i := range typed.ChunkBoundaries {
		edge := CandidateEdges{
			ChunkID: knowledge.ChunkID(fmt.Sprintf("chunk_%d_%d", time.Now().Unix(), i)),
		}

		// Extract references from metadata if available
		if refs, ok := typed.Metadata["references"].([]string); ok {
			edge.References = refs
		}

		edges = append(edges, edge)
	}

	return edges
}

// stage5Admission makes admission decisions.
func (p *Pipeline) stage5Admission(ctx context.Context, typed *TypedIngestion, reports []ScannerReport) (IngestDisposition, DispositionReason) {
	if p.evaluator == nil || p.policy == nil {
		return DispositionCommit, DispositionReason{Stage: "admission", Explanation: "no policy configured"}
	}

	// Check trust class
	trustClass := p.policy.DefaultTrustClass
	admitted, reason := p.evaluator.AdmitTrustClass(trustClass)
	if !admitted {
		return DispositionReject, DispositionReason{Stage: "admission", Explanation: reason}
	}

	// Check suspicion scores across all chunks
	maxSuspicion := 0.0
	for _, report := range reports {
		if report.SuspicionScore > maxSuspicion {
			maxSuspicion = report.SuspicionScore
		}
	}

	// Check quota
	remaining, _ := p.evaluator.QuotaRemaining(p.raw.SourcePrincipal)
	if remaining == 0 {
		return DispositionQuarantine, DispositionReason{Stage: "admission", Explanation: "quota exceeded"}
	}

	// If high suspicion, quarantine
	if maxSuspicion > 0.7 {
		return DispositionQuarantine, DispositionReason{Stage: "admission", Explanation: fmt.Sprintf("high suspicion score: %.2f", maxSuspicion)}
	}

	return DispositionCommit, DispositionReason{Stage: "admission", Explanation: "passed all checks"}
}

type commitResult struct {
	committed      int
	quarantined    int
	chunkIDs       []knowledge.ChunkID
	edgesCreated   int
	eventsEmitted  int
	quarantinePath string
}

// stage6Commit commits chunks to the store.
func (p *Pipeline) stage6Commit(ctx context.Context, typed *TypedIngestion, edges []CandidateEdges, disposition IngestDisposition) (*commitResult, error) {
	result := &commitResult{}

	if disposition == DispositionReject {
		return result, nil
	}

	// Quarantine if needed
	if disposition == DispositionQuarantine {
		quarantinePath, err := p.quarantineContent(ctx, typed)
		if err != nil {
			return result, fmt.Errorf("quarantine: %w", err)
		}
		result.quarantinePath = quarantinePath
		result.quarantined = len(typed.ChunkBoundaries)

		// Emit quarantine event
		// (would use event log in real implementation)
		result.eventsEmitted++

		return result, nil
	}

	// Commit chunks
	for i, boundary := range typed.ChunkBoundaries {
		chunkContent := string(typed.Content[boundary.Start:boundary.End])
		chunk := knowledge.KnowledgeChunk{
			ID:                knowledge.ChunkID(fmt.Sprintf("chunk_%d_%d", time.Now().UnixNano(), i)),
			MemoryClass:       knowledge.MemoryClassWorking,
			SourceOrigin:      knowledge.SourceOriginFile,
			SourcePrincipal:   p.raw.SourcePrincipal,
			AcquisitionMethod: knowledge.AcquisitionMethod(p.raw.AcquisitionMethod),
			AcquiredAt:        p.raw.AcquiredAt,
			TrustClass:        p.policy.DefaultTrustClass,
			Body: knowledge.ChunkBody{
				Raw: chunkContent,
				Fields: map[string]any{
					"content_type": typed.ContentType,
				},
			},
		}

		_, err := p.store.Save(chunk)
		if err != nil {
			return result, fmt.Errorf("save chunk: %w", err)
		}

		result.chunkIDs = append(result.chunkIDs, chunk.ID)
		result.committed++

		// Create edges if any
		if i < len(edges) {
			for _, targetID := range edges[i].CallsTo {
				edge := knowledge.ChunkEdge{
					FromChunk: chunk.ID,
					ToChunk:   targetID,
					Kind:      "calls",
				}
				_, err := p.store.SaveEdge(edge)
				if err != nil {
					return result, fmt.Errorf("save edge: %w", err)
				}
				result.edgesCreated++
			}
		}

		// Emit chunk committed event
		result.eventsEmitted++
	}

	return result, nil
}

// quarantineContent writes content to quarantine directory.
func (p *Pipeline) quarantineContent(ctx context.Context, typed *TypedIngestion) (string, error) {
	timestamp := time.Now().UTC().Format("20060102_150405")
	hash := fmt.Sprintf("%x", time.Now().UnixNano())[:8]
	quarantineDir := filepath.Join(p.quarantineDir, fmt.Sprintf("%s_%s", timestamp, hash))

	if err := os.MkdirAll(quarantineDir, 0750); err != nil {
		return "", fmt.Errorf("create quarantine dir: %w", err)
	}

	// Write content
	contentPath := filepath.Join(quarantineDir, "content.bin")
	if err := os.WriteFile(contentPath, typed.Content, 0640); err != nil {
		return "", fmt.Errorf("write quarantine content: %w", err)
	}

	// Write reason
	reasonPath := filepath.Join(quarantineDir, "reason.txt")
	reason := fmt.Sprintf("Quarantined at %s\nContent type: %s\nChunks: %d\n",
		time.Now().UTC().Format(time.RFC3339),
		typed.ContentType,
		len(typed.ChunkBoundaries))
	if err := os.WriteFile(reasonPath, []byte(reason), 0640); err != nil {
		return "", fmt.Errorf("write quarantine reason: %w", err)
	}

	return quarantineDir, nil
}

func detectMIMEType(path string, content []byte) string {
	if path != "" {
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			return "text/x-go"
		case ".py":
			return "text/x-python"
		case ".rs":
			return "text/x-rust"
		case ".js", ".ts", ".tsx":
			return "text/x-javascript"
		case ".md":
			return "text/markdown"
		case ".txt":
			return "text/plain"
		case ".json":
			return "application/json"
		case ".yaml", ".yml":
			return "application/yaml"
		}
	}

	// Simple content detection
	if len(content) > 0 {
		// Check for common binary signatures
		if content[0] == 0x7f && len(content) > 1 && content[1] == 'E' && content[2] == 'L' && content[3] == 'F' {
			return "application/x-elf"
		}
		if content[0] == 0x89 && len(content) > 3 && string(content[1:4]) == "PNG" {
			return "image/png"
		}
	}

	return "application/octet-stream"
}

func isBinaryContent(content []byte) bool {
	// Simple check: look for null bytes or high ratio of non-printable chars
	nonPrintable := 0
	for _, b := range content {
		if b == 0 {
			return true // Null byte = binary
		}
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			nonPrintable++
		}
	}

	// If more than 10% non-printable, consider binary
	if len(content) > 0 && float64(nonPrintable)/float64(len(content)) > 0.1 {
		return true
	}

	return false
}
