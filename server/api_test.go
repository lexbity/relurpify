package server

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/stretchr/testify/assert"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubAgent struct{}

func (stubAgent) Initialize(config *core.Config) error { return nil }
func (stubAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	state.Set("handled", true)
	return &core.Result{NodeID: "stub", Success: true, Data: map[string]interface{}{"ok": true}}, nil
}
func (stubAgent) Capabilities() []core.Capability { return nil }
func (stubAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	return graph.NewGraph(), nil
}

func TestAPIServerHandleTask(t *testing.T) {
	api := &APIServer{
		Agent:   stubAgent{},
		Context: core.NewContext(),
		Logger:  log.New(io.Discard, "", 0),
	}
	reqBody, _ := json.Marshal(TaskRequest{
		Instruction: "test",
		Type:        core.TaskTypeAnalysis,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/task", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	api.handleTask(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp TaskResponse
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "stub", resp.Result.NodeID)
}
