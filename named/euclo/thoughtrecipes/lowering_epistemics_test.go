package thoughtrecipe

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoweringCarriesEpistemicsAnnotation(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe lowering_epistemics
"E."
trigger as capability:
  may read workspace
input prompt: user.prompt
agent reviewer uses react
run reviewer:
  capture:
    input.prompt : Text as given -> state.answer
    findings -> state.findings
`)
	plan, err := LowerDocument(doc)
	require.NoError(t, err)
	require.Len(t, plan.Steps, 1)
	bindings := plan.Steps[0].CaptureBindings
	require.Len(t, bindings, 2)
	require.NotNil(t, bindings[0].Epistemics)
	require.Equal(t, "given", bindings[0].Epistemics.Value)
	require.Nil(t, bindings[1].Epistemics, "existing recipe shapes remain byte-identical")
}
