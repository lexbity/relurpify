// Command critic-runner executes the critic ensemble offline, end to end,
// with telemetry on the table. It is the book's first production importer
// of package critic: a small main that builds the ensemble graph from
// scripted models (the data plane — the same pattern as the framework's
// own offline backend), runs it over a fresh envelope, and prints the
// graph's telemetry and the pipeline's outcome.
package main

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/cognitionzoo/critic"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// offlineModel is a deterministic scripted model for the runner — model
// output is data, not an action, so a scripted model cheats nothing (the
// framework itself ships offline and tape backends for exactly this
// purpose). It replays its scripted responses in order and cycles.
type offlineModel struct {
	responses []string
	next      int
}

func (m *offlineModel) Generate(_ context.Context, _ string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: m.nextResponse()}, nil
}

func (m *offlineModel) GenerateStream(ctx context.Context, prompt string, _ *model.LLMOptions) (<-chan string, error) {
	out := make(chan string)
	response := m.nextResponse()
	go func() {
		defer close(out)
		for _, word := range strings.SplitAfter(response, " ") {
			select {
			case out <- word:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (m *offlineModel) Chat(ctx context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Generate(ctx, "", nil)
}

func (m *offlineModel) ChatWithTools(ctx context.Context, _ []model.Message, _ []model.LLMToolSpec, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Generate(ctx, "", nil)
}

func (m *offlineModel) nextResponse() string {
	response := m.responses[m.next%len(m.responses)]
	m.next++
	return response
}

// printSink puts the graph's telemetry on the table: one line per event.
type printSink struct{}

func (printSink) Emit(event telemetry.Event) {
	line := fmt.Sprintf("telemetry %-20s node=%s", event.Type, event.NodeID)
	if len(event.Metadata) > 0 {
		keys := make([]string, 0, len(event.Metadata))
		for k := range event.Metadata {
			keys = append(keys, k)
		}
		sortStrings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%v", k, event.Metadata[k]))
		}
		line += " " + strings.Join(parts, " ")
	}
	fmt.Println(line)
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func main() {
	// Three independent critics: one flags issues, two pass — the ensemble
	// is conservative, so the revision fires. After the revision, the
	// critics' second scripted turn passes clean.
	cfg := critic.EnsembleConfig{
		DraftModel: &offlineModel{responses: []string{"an initial draft with rough edges"}},
		CriticModels: []model.LanguageModel{
			&offlineModel{responses: []string{"issues: error handling is missing", "clean"}},
			&offlineModel{responses: []string{"clean", "clean"}},
			&offlineModel{responses: []string{"clean", "clean"}},
		},
		ReviseModel:  &offlineModel{responses: []string{"a revised draft with error handling"}},
		MaxRevisions: 2,
	}

	g, err := critic.BuildEnsembleGraph(cfg)
	if err != nil {
		fmt.Println("build:", err)
		return
	}
	if err := g.SetTelemetry(printSink{}); err != nil {
		fmt.Println("telemetry:", err)
		return
	}

	env := &contextdata.Envelope{}
	env.SetTaskID("critic-runner")

	result, err := g.Execute(context.Background(), env)
	if err != nil {
		fmt.Println("execute:", err)
		return
	}

	fmt.Println()
	fmt.Printf("result:     success=%v node=%s\n", result.Success, result.NodeID)
	verdict, _ := contextdata.GetTyped[string](env, "critic.verdict")
	revisions, _ := contextdata.GetTyped[int](env, "critic.revisions")
	draft, _ := contextdata.GetTyped[string](env, "critic.draft")
	size, _ := contextdata.GetTyped[int](env, "critic.ensemble.size")
	fmt.Printf("ensemble:   size=%d verdict=%s\n", size, verdict)
	fmt.Printf("revisions:  %d\n", revisions)
	fmt.Printf("final draft: %s\n", draft)

	report := g.LastPreflightReport()
	if report != nil {
		fmt.Printf("preflight:  %d issues, %d placements\n", len(report.Issues), len(report.Placements))
	}
}
