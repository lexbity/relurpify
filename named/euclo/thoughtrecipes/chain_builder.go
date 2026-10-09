package thoughtrecipe

import (
	"fmt"
	"strconv"
	"strings"

	chaineragent "codeburg.org/lexbit/relurpify/cognitionzoo/chainer"
)

// buildChainerChain converts the typed `link` directives of a chainer step
// into a chainer.Chain (contract: chainer/link_builds_chain). Each `link:`
// block becomes one Chain step:
//
//   - the link's first text argument is its name;
//   - `from <path>` clauses become input keys fed to the prompt context;
//   - a `prompt` clause (inline text) becomes the system prompt;
//   - a `capture` clause becomes the link's output key on the envelope.
//
// A link block that omits its prompt or its capture destination is an
// authoring error surfaced at execution, never a silently skipped link.
func buildChainerChain(directives []TypedDirective) (*chaineragent.Chain, error) {
	var linkBlocks []TypedDirective
	for _, directive := range directives {
		if directive.Name == "link" {
			linkBlocks = append(linkBlocks, directive)
		}
	}
	if len(linkBlocks) == 0 {
		return nil, fmt.Errorf("chainer step requires at least one `link` block")
	}

	chain := &chaineragent.Chain{Links: make([]chaineragent.Link, 0, len(linkBlocks))}
	for _, block := range linkBlocks {
		link := chaineragent.Link{
			Name: linkBlockName(block),
		}
		for _, item := range block.Body {
			switch item.Name {
			case "from":
				link.InputKeys = append(link.InputKeys, nonEmptyTextArgs(item.TextArgs)...)
			case "prompt":
				prompt := unquoteString(strings.Join(nonEmptyTextArgs(item.TextArgs), " "))
				if strings.TrimSpace(prompt) == "" {
					return nil, fmt.Errorf("chainer link %q requires a non-empty prompt", link.Name)
				}
				link.SystemPrompt = prompt
			case "capture":
				destination := captureDestination(item.TextArgs)
				if destination == "" {
					return nil, fmt.Errorf("chainer link %q requires a capture destination", link.Name)
				}
				link.OutputKey = destination
			}
		}
		if strings.TrimSpace(link.SystemPrompt) == "" {
			return nil, fmt.Errorf("chainer link %q requires a `prompt` clause", link.Name)
		}
		if strings.TrimSpace(link.OutputKey) == "" {
			return nil, fmt.Errorf("chainer link %q requires a `capture` destination", link.Name)
		}
		chain.Links = append(chain.Links, link)
	}
	return chain, nil
}

func linkBlockName(block TypedDirective) string {
	if len(block.TextArgs) > 0 {
		if name := strings.TrimSpace(block.TextArgs[0]); name != "" {
			return name
		}
	}
	return "link"
}

func nonEmptyTextArgs(args []string) []string {
	var out []string
	for _, arg := range args {
		if trimmed := strings.TrimSpace(arg); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// captureDestination extracts the destination side of a capture binding
// rendered as "source -> destination" by the lowering.
func captureDestination(args []string) string {
	for _, arg := range args {
		source, destination, found := strings.Cut(arg, "->")
		if !found {
			continue
		}
		_ = source
		if destination = strings.TrimSpace(destination); destination != "" {
			return destination
		}
	}
	return ""
}

// untilIterationCap parses the react `until` directive's iteration cap into a
// positive budget. A present-but-invalid `until` value is a load error (the
// contract declares ArgsInteger), so this helper never silently ignores a
// malformed directive.
func untilIterationCap(directives []TypedDirective) (int, error) {
	args := DirectiveText(directives, "until")
	if len(args) == 0 {
		return 0, nil
	}
	first := strings.TrimSpace(args[0])
	if first == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(first)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("until directive requires a positive integer, got %q", first)
	}
	return n, nil
}
