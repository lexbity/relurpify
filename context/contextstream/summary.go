package contextstream

// SummaryOf builds the bounded working-state summary of a stream result: the
// shape every context producer writes into working state (FR-15). Bodies and
// chunk content never enter working state — they live on the envelope slice
// where the renderer reads them — so the summary stays far under the 4096-byte
// node-contract bound no matter how large the compilation was.
func SummaryOf(result *Result) map[string]any {
	if result == nil {
		return nil
	}
	summary := map[string]any{
		"request_id": result.Request.ID,
		"epoch":      result.Request.EpochID,
	}
	if result.Compilation != nil {
		summary["chunks"] = len(result.Compilation.StreamedChunks)
		summary["tokens"] = result.Compilation.Record.FinalTokens
	}
	return summary
}
