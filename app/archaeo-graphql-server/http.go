package archaeographqlserver

import (
	"context"
	"encoding/json"
	"net/http"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
)

type AuthorizeFunc func(context.Context, *http.Request) error

type Handler struct {
	Runtime   Runtime
	Schema    *graphql.Schema
	relay     *relay.Handler
	Authorize AuthorizeFunc
}

func NewHandler(runtime Runtime) *Handler {
	root := &rootResolver{runtime: runtime}
	schema := graphql.MustParseSchema(
		SchemaSDL,
		root,
		graphql.UseFieldResolvers(),
		graphql.MaxDepth(12),
		graphql.MaxQueryLength(64*1024),
		graphql.MaxParallelism(32),
	)
	return &Handler{
		Runtime: runtime,
		Schema:  schema,
		relay:   &relay.Handler{Schema: schema},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]any{{"message": "method not allowed"}},
		})
		return
	}
	if h.Authorize != nil {
		if err := h.Authorize(r.Context(), r); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errors": []map[string]any{{"message": err.Error()}},
			})
			return
		}
	}
	h.relay.ServeHTTP(w, r)
}

func (h *Handler) Subscribe(ctx context.Context, query, operationName string, variables map[string]any) (<-chan any, error) {
	return h.Schema.Subscribe(ctx, query, operationName, variables)
}
