package chainer

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the chainer paradigm: a deterministic
// chain of isolated prompt links.
//
// The `link` directive is the only grammar clause the runner honors: each
// `link:` block becomes one Chain step whose `from` keys feed the prompt
// context, whose prompt text or registry PromptID produces the system prompt,
// and whose capture target receives the link output. Nested body items are
// constrained to the declared Body set (from/goal/capture/stream and nested
// directive clauses such as `prompt`); capability and step items inside a
// link block are load errors.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "chainer",
		Summary:  "Deterministic chain of isolated prompt links.",
		Shape:    paradigm.ShapeChain,
		Directives: []paradigm.DirectiveSpec{
			{
				Name:       "link",
				Form:       paradigm.FormBlock,
				Text:       paradigm.ArgNone,
				Repeatable: true,
				Required:   true,
				Body: []paradigm.BodyItem{
					paradigm.BodyItemFrom,
					paradigm.BodyItemGoal,
					paradigm.BodyItemCapture,
					paradigm.BodyItemStream,
					paradigm.BodyItemDirective,
				},
			},
		},
		Guarantees: []string{
			"link blocks execute in declared order",
			"link from keys reach the prompt context on both prompt paths",
			"capture targets receive the link output",
		},
		Conformance: []paradigm.ConformanceCase{
			{
				ID:           "chainer/link_builds_chain",
				Directive:    "link",
				Assertion:    "each `link:` block becomes one executed chain step; the envelope records the executed link count and each capture target holds the link output",
				FixtureShape: "a chainer run block with two `link:` blocks, each with `from`, `prompt`, and a `capture`",
			},
			{
				ID:           "chainer/link_from_registry_prompt",
				Directive:    "link",
				Assertion:    "when a link uses a registry PromptID, the link's `from` keys are injected into the resolved prompt's runtime context state",
				FixtureShape: "a chainer run block with a `link:` block carrying `from state.key` and a registry prompt binding",
			},
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		panic("register chainer paradigm contract: " + err.Error())
	}
}
