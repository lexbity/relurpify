package tui

import (
	"strings"
	"testing"

	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

const workshopSourceFixture = `thoughtrecipe example.flow
"Unicode 界 description"

# Keep this comment exactly.
input prompt: user.request
agent worker uses react # keep agent comment
run worker:
  from input.prompt
  goal """Read 界 source.
Preserve the second line and \backslashes.""" # keep goal comment
  capture result -> state.summary

agent verifier uses react
run verifier:
  from input.prompt
  goal "Check it"
  capture result -> state.verified
`

func TestV2WorkshopSourceEditsPreserveUntouchedBytes(t *testing.T) {
	document, err := thoughtrecipe.ParseSource("flow.erpe", workshopSourceFixture)
	if err != nil {
		t.Fatal(err)
	}
	steps := recipeSteps(document)
	if len(steps) != 2 || steps[0].captures[0] != "state.summary" {
		t.Fatalf("real AST extraction failed: %+v", steps)
	}
	strategy, err := patchRecipeStrategy(workshopSourceFixture, steps[0], "reflection")
	if err != nil || strategy != strings.Replace(workshopSourceFixture, "uses react # keep", "uses reflection # keep", 1) {
		t.Fatalf("strategy patch changed unrelated bytes: %v\n%s", err, strategy)
	}
	goal, err := patchRecipeGoal(workshopSourceFixture, steps[0], "New 界 goal\nsecond line")
	want := strings.Replace(workshopSourceFixture, steps[0].goalClause.Text.Raw, `"""New 界 goal`+"\n"+`second line"""`, 1)
	if err != nil || goal != want {
		t.Fatalf("multiline/Unicode goal patch was not exact: %v\ngot %s\nwant %s", err, goal, want)
	}
	connection, err := patchRecipeConnection(workshopSourceFixture, steps[1], "state.summary")
	want = workshopSourceFixture[:strings.LastIndex(workshopSourceFixture, "from input.prompt")] + strings.Replace(workshopSourceFixture[strings.LastIndex(workshopSourceFixture, "from input.prompt"):], "from input.prompt", "from state.summary", 1)
	if err != nil || connection != want {
		t.Fatalf("connection patch changed unrelated source: %v", err)
	}
	bound, err := patchRecipePrompt(workshopSourceFixture, document, steps[0], "demo.prompt.repair")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := thoughtrecipe.ParseSource("flow.erpe", bound)
	if err != nil {
		t.Fatal(err)
	}
	if recipeSteps(parsed)[0].binding != "demo.prompt.repair" || !strings.Contains(bound, "# keep goal comment") {
		t.Fatal("prompt binding lost import identity or adjacent source")
	}
}

func TestV2WorkshopConnectionsExcludeFutureCaptures(t *testing.T) {
	document, err := thoughtrecipe.ParseSource("flow.erpe", workshopSourceFixture)
	if err != nil {
		t.Fatal(err)
	}
	first := strings.Join(recipeConnections(document, 0), ",")
	second := strings.Join(recipeConnections(document, 1), ",")
	if strings.Contains(first, "state.summary") || !strings.Contains(second, "state.summary") || strings.Contains(second, "state.verified") {
		t.Fatal("connection chooser offered future/self captures")
	}
}
