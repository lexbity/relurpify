package promptprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

func TestEnvelopeGetOnNilEnvelope(t *testing.T) {
	val, ok := envelopeGet(nil, "any.key")
	assert.False(t, ok)
	assert.Nil(t, val)
}

func TestEnvelopeGetStringCoercesNonStrings(t *testing.T) {
	env := newEnvelope(t,
		"count", 42,
		"padding", "  padded  ",
		"nilval", nil,
	)
	assert.Equal(t, "42", envelopeGetString(env, "count"))
	assert.Equal(t, "padded", envelopeGetString(env, "padding"))
	assert.Equal(t, "", envelopeGetString(env, "nilval"), "stored nil must read as empty string")
	assert.Equal(t, "", envelopeGetString(env, "missing"))
	assert.Equal(t, "", envelopeGetString(nil, "missing"))
}

func TestExtractStringFieldFromStructViaJSONRoundTrip(t *testing.T) {
	type record struct {
		Summary string   `json:"summary"`
		Source  string   `json:"source"`
		Tags    []string `json:"tags"`
	}
	rec := record{Summary: " round-tripped ", Source: "graph"}
	assert.Equal(t, "round-tripped", extractStringField(rec, "summary"))
	assert.Equal(t, "graph", extractStringField(rec, "source"))
	assert.Equal(t, "", extractStringField(rec, "missing"))
	assert.Equal(t, "", extractStringField(nil, "summary"))
}

func TestExtractStringFieldMapFastPath(t *testing.T) {
	m := map[string]any{"summary": " direct ", "n": 7}
	assert.Equal(t, "direct", extractStringField(m, "summary"))
	assert.Equal(t, "7", extractStringField(m, "n"))
	assert.Equal(t, "", extractStringField(m, "missing"))
}

func TestExtractStringFieldUnmarshalableValueYieldsEmpty(t *testing.T) {
	assert.Equal(t, "", extractStringField(make(chan int), "summary"))
}

func TestToSliceOfAny(t *testing.T) {
	assert.Nil(t, toSliceOfAny(nil))

	direct := toSliceOfAny([]any{"a", "b"})
	assert.Equal(t, []any{"a", "b"}, direct)

	// Non-[]any slices round-trip through JSON.
	converted := toSliceOfAny([]string{"x", "y"})
	assert.Equal(t, []any{"x", "y"}, converted)

	assert.Nil(t, toSliceOfAny("not a slice"))
	assert.Nil(t, toSliceOfAny(make(chan int)))
}

func TestEnvelopeSetTypedIsReadableAsAny(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	contextdata.SetTyped[any](env, "k", "v")
	raw, ok := envelopeGet(env, "k")
	assert.True(t, ok)
	assert.Equal(t, "v", raw)
}
