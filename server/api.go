package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/persistence"
	"log"
	"net/http"
	"path"
	"strings"
	"time"
)

// APIServer exposes HTTP endpoints for testing agents without an editor.
type APIServer struct {
	Agent             graph.Agent
	Context           *core.Context
	Logger            *log.Logger
	WorkflowStatePath string
}

// TaskRequest describes incoming API payload.
type TaskRequest struct {
	Instruction string                 `json:"instruction"`
	Type        core.TaskType          `json:"type"`
	Context     map[string]interface{} `json:"context"`
}

// TaskResponse describes API response.
type TaskResponse struct {
	Result *core.Result `json:"result"`
	Error  string       `json:"error,omitempty"`
}

type workflowActionRequest struct {
	Instruction string `json:"instruction,omitempty"`
	StepID      string `json:"step_id,omitempty"`
}

// Serve starts listening on the provided address.
func (s *APIServer) Serve(addr string) error {
	return s.ServeContext(context.Background(), addr)
}

// ServeContext allows the caller to control shutdown via context cancellation.
func (s *APIServer) ServeContext(ctx context.Context, addr string) error {
	server := s.newHTTPServer(addr)
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()
	if s.Logger != nil {
		s.Logger.Printf("API listening on %s", addr)
	}
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *APIServer) newHTTPServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/task", s.handleTask)
	mux.HandleFunc("/api/context", s.handleContext)
	mux.HandleFunc("/api/workflows", s.handleWorkflows)
	mux.HandleFunc("/api/workflows/", s.handleWorkflowByID)
	return &http.Server{
		Addr:    addr,
		Handler: mux,
	}
}

func (s *APIServer) handleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req TaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		req.Type = core.TaskTypeCodeModification
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	task := &core.Task{
		ID:          time.Now().Format("20060102150405"),
		Type:        req.Type,
		Instruction: req.Instruction,
		Context:     req.Context,
	}
	state := s.Context.Clone()
	scope := task.ID
	if scope != "" {
		defer state.ClearHandleScope(scope)
	}
	result, err := s.Agent.Execute(ctx, task, state)
	resp := TaskResponse{Result: result}
	if err != nil {
		resp.Error = err.Error()
	}
	if err == nil {
		s.Context.Merge(state)
	}
	writeJSON(w, resp)
}

func (s *APIServer) handleContext(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Context)
}

func (s *APIServer) handleWorkflows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	store, err := s.openWorkflowStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer store.Close()
	workflows, err := store.ListWorkflows(r.Context(), 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, workflows)
}

func (s *APIServer) handleWorkflowByID(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/workflows/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	parts := strings.Split(trimmed, "/")
	workflowID := path.Clean(parts[0])
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	store, err := s.openWorkflowStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer store.Close()

	switch {
	case action == "" && r.Method == http.MethodGet:
		s.handleWorkflowInspect(w, r, store, workflowID)
	case action == "steps" && r.Method == http.MethodGet:
		steps, err := store.ListSteps(r.Context(), workflowID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, steps)
	case action == "events" && r.Method == http.MethodGet:
		events, err := store.ListEvents(r.Context(), workflowID, 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, events)
	case action == "facts" && r.Method == http.MethodGet:
		s.handleWorkflowKnowledge(w, r, store, workflowID, persistence.KnowledgeKindFact)
	case action == "issues" && r.Method == http.MethodGet:
		s.handleWorkflowKnowledge(w, r, store, workflowID, persistence.KnowledgeKindIssue)
	case action == "decisions" && r.Method == http.MethodGet:
		s.handleWorkflowKnowledge(w, r, store, workflowID, persistence.KnowledgeKindDecision)
	case action == "resume" && r.Method == http.MethodPost:
		s.handleWorkflowAction(w, r, store, workflowID, map[string]any{"workflow_id": workflowID})
	case action == "cancel" && r.Method == http.MethodPost:
		_, err := store.UpdateWorkflowStatus(r.Context(), workflowID, 0, persistence.WorkflowRunStatusCanceled, "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "workflow_id": workflowID, "status": "canceled"})
	case action == "rerun-step" && r.Method == http.MethodPost:
		var req workflowActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.StepID) == "" {
			http.Error(w, "step_id required", http.StatusBadRequest)
			return
		}
		s.handleWorkflowAction(w, r, store, workflowID, map[string]any{
			"workflow_id":        workflowID,
			"rerun_from_step_id": strings.TrimSpace(req.StepID),
		})
	case action == "rerun-invalidated" && r.Method == http.MethodPost:
		s.handleWorkflowAction(w, r, store, workflowID, map[string]any{
			"workflow_id":       workflowID,
			"rerun_invalidated": true,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *APIServer) handleWorkflowInspect(w http.ResponseWriter, r *http.Request, store *persistence.SQLiteWorkflowStateStore, workflowID string) {
	workflow, ok, err := store.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	steps, err := store.ListSteps(r.Context(), workflowID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	events, err := store.ListEvents(r.Context(), workflowID, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"workflow": workflow,
		"steps":    steps,
		"events":   events,
	})
}

func (s *APIServer) handleWorkflowKnowledge(w http.ResponseWriter, r *http.Request, store *persistence.SQLiteWorkflowStateStore, workflowID string, kind persistence.KnowledgeKind) {
	records, err := store.ListKnowledge(r.Context(), workflowID, kind, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, records)
}

func (s *APIServer) handleWorkflowAction(w http.ResponseWriter, r *http.Request, store *persistence.SQLiteWorkflowStateStore, workflowID string, metadata map[string]any) {
	workflow, ok, err := store.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	task := &core.Task{
		ID:          fmt.Sprintf("api-%d", time.Now().UnixNano()),
		Type:        workflow.TaskType,
		Instruction: workflow.Instruction,
		Context:     metadata,
	}
	state := s.Context.Clone()
	state.Set("task.id", task.ID)
	state.Set("task.type", string(task.Type))
	state.Set("task.instruction", task.Instruction)
	result, err := s.Agent.Execute(r.Context(), task, state)
	resp := TaskResponse{Result: result}
	if err != nil {
		resp.Error = err.Error()
	}
	writeJSON(w, resp)
}

func (s *APIServer) openWorkflowStore() (*persistence.SQLiteWorkflowStateStore, error) {
	if strings.TrimSpace(s.WorkflowStatePath) == "" {
		return nil, fmt.Errorf("workflow state path not configured")
	}
	return persistence.NewSQLiteWorkflowStateStore(s.WorkflowStatePath)
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
