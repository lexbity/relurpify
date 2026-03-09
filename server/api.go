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
	Inspector         Inspector
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
	mux.HandleFunc("/api/capabilities", s.handleCapabilities)
	mux.HandleFunc("/api/capabilities/", s.handleCapabilityByID)
	mux.HandleFunc("/api/prompts", s.handlePrompts)
	mux.HandleFunc("/api/prompts/", s.handlePromptByID)
	mux.HandleFunc("/api/providers", s.handleProviders)
	mux.HandleFunc("/api/providers/", s.handleProviderByID)
	mux.HandleFunc("/api/resources", s.handleResources)
	mux.HandleFunc("/api/resources/", s.handleResourceByID)
	mux.HandleFunc("/api/workflow-resources/read", s.handleWorkflowResourceRead)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/sessions/", s.handleSessionByID)
	mux.HandleFunc("/api/approvals", s.handleApprovals)
	mux.HandleFunc("/api/approvals/", s.handleApprovalByID)
	mux.HandleFunc("/api/workflows", s.handleWorkflows)
	mux.HandleFunc("/api/workflows/", s.handleWorkflowByID)
	return &http.Server{
		Addr:    addr,
		Handler: mux,
	}
}

func (s *APIServer) handlePrompts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "prompt inspection unavailable", http.StatusNotImplemented)
		return
	}
	prompts, err := s.Inspector.ListPrompts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, prompts)
}

func (s *APIServer) handlePromptByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "prompt inspection unavailable", http.StatusNotImplemented)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/prompts/"), "/")
	resource, err := s.Inspector.GetPrompt(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, resource)
}

func (s *APIServer) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "capability inspection unavailable", http.StatusNotImplemented)
		return
	}
	capabilities, err := s.Inspector.ListCapabilities(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, capabilities)
}

func (s *APIServer) handleCapabilityByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "capability inspection unavailable", http.StatusNotImplemented)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/capabilities/"), "/")
	resource, err := s.Inspector.GetCapability(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, resource)
}

func (s *APIServer) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "provider inspection unavailable", http.StatusNotImplemented)
		return
	}
	providers, err := s.Inspector.ListProviders(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, providers)
}

func (s *APIServer) handleProviderByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "provider inspection unavailable", http.StatusNotImplemented)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/providers/"), "/")
	resource, err := s.Inspector.GetProvider(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, resource)
}

func (s *APIServer) handleResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "resource inspection unavailable", http.StatusNotImplemented)
		return
	}
	resources, err := s.Inspector.ListResources(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resources)
}

func (s *APIServer) handleResourceByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "resource inspection unavailable", http.StatusNotImplemented)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/resources/"), "/")
	resource, err := s.Inspector.GetResource(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, resource)
}

func (s *APIServer) handleWorkflowResourceRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "workflow resource inspection unavailable", http.StatusNotImplemented)
		return
	}
	uri := strings.TrimSpace(r.URL.Query().Get("uri"))
	if uri == "" {
		http.Error(w, "uri query parameter required", http.StatusBadRequest)
		return
	}
	resource, err := s.Inspector.GetWorkflowResource(r.Context(), uri)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, resource)
}

func (s *APIServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "session inspection unavailable", http.StatusNotImplemented)
		return
	}
	sessions, err := s.Inspector.ListSessions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, sessions)
}

func (s *APIServer) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "session inspection unavailable", http.StatusNotImplemented)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sessions/"), "/")
	resource, err := s.Inspector.GetSession(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, resource)
}

func (s *APIServer) handleApprovals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "approval inspection unavailable", http.StatusNotImplemented)
		return
	}
	approvals, err := s.Inspector.ListApprovals(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, approvals)
}

func (s *APIServer) handleApprovalByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.Inspector == nil {
		http.Error(w, "approval inspection unavailable", http.StatusNotImplemented)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/approvals/"), "/")
	resource, err := s.Inspector.GetApproval(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, resource)
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
	case action == "delegations" && r.Method == http.MethodGet:
		s.handleWorkflowDelegations(w, r, store, workflowID)
	case action == "artifacts" && r.Method == http.MethodGet:
		s.handleWorkflowArtifacts(w, r, store, workflowID)
	case action == "providers" && r.Method == http.MethodGet:
		s.handleWorkflowProviders(w, r, store, workflowID)
	case action == "sessions" && r.Method == http.MethodGet:
		s.handleWorkflowSessions(w, r, store, workflowID)
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

func (s *APIServer) handleWorkflowDelegations(w http.ResponseWriter, r *http.Request, store *persistence.SQLiteWorkflowStateStore, workflowID string) {
	records, err := store.ListDelegations(r.Context(), workflowID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]DelegationResource, 0, len(records))
	for _, record := range records {
		insertionAction := ""
		if record.Result != nil {
			insertionAction = string(record.Result.Insertion.Action)
		}
		targetTitle := record.Request.TargetCapabilityID
		if targetTitle == "" {
			targetTitle = record.Request.TargetProviderID
		}
		out = append(out, DelegationResource{
			Meta: InspectableMeta{
				ID:         record.DelegationID,
				Kind:       "delegation",
				Title:      targetTitle,
				TrustClass: string(record.TrustClass),
				Source:     record.Request.TargetProviderID,
				State:      string(record.State),
				CapturedAt: record.UpdatedAt.Format(time.RFC3339),
			},
			Delegation: DelegationPayload{
				DelegationID:       record.DelegationID,
				RunID:              record.RunID,
				TaskID:             record.TaskID,
				State:              string(record.State),
				TargetCapabilityID: record.Request.TargetCapabilityID,
				TargetProviderID:   record.Request.TargetProviderID,
				TargetSessionID:    record.Request.TargetSessionID,
				Recoverability:     string(record.Recoverability),
				InsertionAction:    insertionAction,
				ResourceRefs:       append([]string(nil), record.Request.ResourceRefs...),
			},
		})
	}
	writeJSON(w, out)
}

func (s *APIServer) handleWorkflowArtifacts(w http.ResponseWriter, r *http.Request, store *persistence.SQLiteWorkflowStateStore, workflowID string) {
	records, err := store.ListWorkflowArtifacts(r.Context(), workflowID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]ArtifactResource, 0, len(records))
	for _, record := range records {
		out = append(out, ArtifactResource{
			Meta: InspectableMeta{
				ID:         record.ArtifactID,
				Kind:       "artifact",
				Title:      record.Kind,
				State:      record.ContentType,
				CapturedAt: record.CreatedAt.Format(time.RFC3339),
			},
			Artifact: ArtifactPayload{
				ArtifactID:  record.ArtifactID,
				RunID:       record.RunID,
				Kind:        record.Kind,
				ContentType: record.ContentType,
				SummaryText: record.SummaryText,
			},
		})
	}
	writeJSON(w, out)
}

func (s *APIServer) handleWorkflowProviders(w http.ResponseWriter, r *http.Request, store *persistence.SQLiteWorkflowStateStore, workflowID string) {
	runIDs, err := workflowRunIDs(r.Context(), store, workflowID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := []ProviderResource{}
	for _, runID := range runIDs {
		records, err := store.ListProviderSnapshots(r.Context(), workflowID, runID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, record := range records {
			out = append(out, ProviderResource{
				Meta: InspectableMeta{
					ID:         record.ProviderID,
					Kind:       string(record.Descriptor.Kind),
					Title:      record.ProviderID,
					TrustClass: string(record.Descriptor.TrustBaseline),
					Source:     record.Descriptor.ConfiguredSource,
					State:      record.Health.Status,
					CapturedAt: record.CapturedAt.Format(time.RFC3339),
				},
				Provider: ProviderPayload{
					ProviderID:     record.ProviderID,
					ProviderKind:   string(record.Descriptor.Kind),
					TrustBaseline:  string(record.Descriptor.TrustBaseline),
					Recoverability: string(record.Recoverability),
					ConfiguredFrom: record.Descriptor.ConfiguredSource,
					CapabilityIDs:  append([]string(nil), record.CapabilityIDs...),
					Metadata:       summarizeAnyMap(record.Metadata),
				},
			})
		}
	}
	writeJSON(w, out)
}

func (s *APIServer) handleWorkflowSessions(w http.ResponseWriter, r *http.Request, store *persistence.SQLiteWorkflowStateStore, workflowID string) {
	runIDs, err := workflowRunIDs(r.Context(), store, workflowID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := []SessionResource{}
	for _, runID := range runIDs {
		records, err := store.ListProviderSessionSnapshots(r.Context(), workflowID, runID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, record := range records {
			out = append(out, SessionResource{
				Meta: InspectableMeta{
					ID:         record.Session.ID,
					Kind:       "workflow-session-snapshot",
					Title:      record.Session.ID,
					TrustClass: string(record.Session.TrustClass),
					Source:     record.Session.ProviderID,
					State:      record.Session.Health,
					CapturedAt: record.CapturedAt.Format(time.RFC3339),
				},
				Session: SessionPayload{
					SessionID:       record.Session.ID,
					ProviderID:      record.Session.ProviderID,
					WorkflowID:      record.Session.WorkflowID,
					TaskID:          record.Session.TaskID,
					Recoverability:  string(record.Session.Recoverability),
					CapabilityIDs:   append([]string(nil), record.Session.CapabilityIDs...),
					LastActivityAt:  record.Session.LastActivityAt,
					MetadataSummary: summarizeInterfaceMap(record.Session.Metadata),
				},
			})
		}
	}
	writeJSON(w, out)
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

func workflowRunIDs(ctx context.Context, store *persistence.SQLiteWorkflowStateStore, workflowID string) ([]string, error) {
	runIDs := map[string]struct{}{}
	delegations, err := store.ListDelegations(ctx, workflowID, "")
	if err != nil {
		return nil, err
	}
	for _, delegation := range delegations {
		if strings.TrimSpace(delegation.RunID) != "" {
			runIDs[delegation.RunID] = struct{}{}
		}
	}
	artifacts, err := store.ListWorkflowArtifacts(ctx, workflowID, "")
	if err != nil {
		return nil, err
	}
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.RunID) != "" {
			runIDs[artifact.RunID] = struct{}{}
		}
	}
	out := make([]string, 0, len(runIDs))
	for runID := range runIDs {
		out = append(out, runID)
	}
	return out, nil
}

func summarizeAnyMap(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, fmt.Sprintf("%s=%v", key, value))
	}
	return out
}

func summarizeInterfaceMap(values map[string]interface{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, fmt.Sprintf("%s=%v", key, value))
	}
	return out
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
