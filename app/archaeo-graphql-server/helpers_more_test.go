package archaeographqlserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMapHelpersAndKeyNormalization(t *testing.T) {
	var m Map
	require.NoError(t, m.UnmarshalGraphQL(map[string]any{"hello_world": 1}))
	require.Equal(t, Map{"hello_world": 1}, m)
	require.True(t, (*Map)(nil).ImplementsGraphQLType("Map"))
	require.False(t, (*Map)(nil).ImplementsGraphQLType("String"))

	mapped, err := toMap(struct {
		HelloWorld string
	}{HelloWorld: "ok"})
	require.NoError(t, err)
	require.Equal(t, "ok", (*mapped)["helloWorld"])

	maps, err := toMaps([]struct {
		HelloWorld string
	}{{HelloWorld: "ok"}})
	require.NoError(t, err)
	require.Len(t, maps, 1)
	require.Equal(t, "ok", maps[0]["helloWorld"])

	require.Equal(t, "fooBarBaz", graphqlKey("fooBarBaz"))
	require.Equal(t, "fooBarBaz", strings.Join(splitGraphQLKey("fooBarBaz"), ""))
	require.Equal(t, 0, argLimit(nil))
	v := int32(4)
	require.Equal(t, 4, argLimit(&v))
}

func TestCloneAndResolverHelpers(t *testing.T) {
	input := Map{"nested": Map{"value": "x"}}
	cloned := cloneMapAny(input)
	require.IsType(t, map[string]any{}, cloned)
	require.Equal(t, Map{"value": "x"}, cloned["nested"])
	clonedPtr := cloneMapAnyPtr(&input)
	require.Equal(t, cloned, clonedPtr)

	ids := []string{"a", "b"}
	require.Equal(t, []string{"a", "b"}, idSlice(&ids))
	require.Nil(t, idSlice(nil))
	require.Equal(t, 7, *intPtr32(ptr32(7)))
	now := time.Now().UTC().Truncate(time.Second)
	require.Equal(t, now, *timePtr(now))

	root := &rootResolver{runtime: Runtime{}}
	require.NotNil(t, root.Query())
	require.NotNil(t, root.Mutation())
	require.NotNil(t, root.Subscription())
}

func TestHandlerAuthorizeAndMethodGuards(t *testing.T) {
	h := NewHandler(Runtime{})
	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	h.Authorize = func(context.Context, *http.Request) error { return context.Canceled }
	req = httptest.NewRequest(http.MethodPost, "/graphql", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func ptr32(v int32) *int32 { return &v }
