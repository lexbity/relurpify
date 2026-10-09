package thoughtrecipe

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func firstCaptureBlockFromDoc(t *testing.T, doc *ThoughtRecipeDocument) *CaptureBlock {
	t.Helper()
	for _, decl := range doc.Declarations {
		run, ok := decl.(*RunDecl)
		if !ok {
			continue
		}
		for _, item := range run.Items {
			if block, ok := item.(*CaptureBlock); ok {
				return block
			}
		}
	}
	t.Fatal("no capture block found in document")
	return nil
}

func TestParseCaptureEpistemicsAnnotation(t *testing.T) {
	block := firstCaptureBlockFromDoc(t, mustParseDoc(t, `thoughtrecipe epistemics
"E."
trigger as capability:
  may read workspace
input prompt: user.prompt
agent reviewer uses react
run reviewer:
  capture:
    input.prompt : Text as given -> state.answer
    findings : ReviewFindings as claimed -> state.findings
    plain -> state.plain
`))
	require.Len(t, block.Bindings, 3)
	require.NotNil(t, block.Bindings[0].Epistemics)
	require.Equal(t, "given", block.Bindings[0].Epistemics.Value)
	require.Equal(t, "given", block.Bindings[0].EpistemicsValue())
	require.NotNil(t, block.Bindings[1].Epistemics)
	require.Equal(t, "claimed", block.Bindings[1].Epistemics.Value)
	require.Nil(t, block.Bindings[2].Epistemics, "un-annotated capture defaults to nil")
	require.Equal(t, "claimed", block.Bindings[2].EpistemicsValue())
}

func TestParseCaptureEpistemicsUnknownAnnotationFails(t *testing.T) {
	_, err := ParseSource("bad_epistemics.erpe", `thoughtrecipe bad
trigger as capability:
  may read workspace
agent reviewer uses react
run reviewer:
  capture:
    input.prompt : Text as verified -> state.answer
`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected epistemic annotation (claimed|given)")
}

func TestParseCaptureEpistemicsDanglingAsFails(t *testing.T) {
	_, err := ParseSource("bad_as.erpe", `thoughtrecipe badas
trigger as capability:
  may read workspace
agent reviewer uses react
run reviewer:
  capture:
    input.prompt : Text as -> state.answer
`)
	require.Error(t, err)
}

func TestSemanticGivenRequiresUserOriginSource(t *testing.T) {
	t.Run("rejects state source", func(t *testing.T) {
		doc := mustParseDoc(t, `thoughtrecipe sem
trigger as capability:
  may read workspace
agent reviewer uses react
run reviewer:
  capture:
    findings : Text as given -> state.answer
`)
		err := NewSymbolTable(doc).Resolve()
		require.Error(t, err)
		require.Contains(t, err.Error(), "'as given' requires a user-origin source")
	})

	t.Run("accepts declared user input", func(t *testing.T) {
		doc := mustParseDoc(t, `thoughtrecipe sem2
trigger as capability:
  may read workspace
input prompt: user.prompt
agent reviewer uses react
run reviewer:
  capture:
    input.prompt : Text as given -> state.answer
`)
		require.NoError(t, NewSymbolTable(doc).Resolve())
	})

	t.Run("accepts explicit user path", func(t *testing.T) {
		doc := mustParseDoc(t, `thoughtrecipe sem3
trigger as capability:
  may read workspace
agent reviewer uses react
run reviewer:
  capture:
    user.prompt : Text as given -> state.answer
`)
		require.NoError(t, NewSymbolTable(doc).Resolve())
	})
}

func TestSemanticGivenRejectsNonUserInput(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe sem4
trigger as capability:
  may read workspace
input workspace: "**/*.go"
agent reviewer uses react
run reviewer:
  capture:
    input.workspace : Text as given -> state.answer
`)
	err := NewSymbolTable(doc).Resolve()
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "sem4:") || strings.Contains(err.Error(), "as given"), "error must carry position and reason: %v", err)
}
