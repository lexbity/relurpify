package prompt

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	gmtext "github.com/yuin/goldmark/text"
)

const (
	Str17460_markdown = "```"
)


var markdownParser = goldmark.New(
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
)

func renderMarkdownBody(body string, vars map[string]string) (string, error) {
	source := []byte(body)
	doc := markdownParser.Parser().Parse(gmtext.NewReader(source))

	var out strings.Builder
	if err := renderBlockChildren(&out, doc, source, vars); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func renderBlockChildren(out *strings.Builder, parent ast.Node, source []byte, vars map[string]string) error {
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		if err := renderBlockNode(out, child, source, vars); err != nil {
			return err
		}
	}
	return nil
}

func renderBlockNode(out *strings.Builder, node ast.Node, source []byte, vars map[string]string) error {
	switch n := node.(type) {
	case *ast.Document:
		return renderBlockChildren(out, n, source, vars)
	case *ast.Heading:
		out.WriteString(strings.Repeat("#", n.Level))
		out.WriteByte(' ')
		text, err := renderInlineChildren(n, source, vars)
		if err != nil {
			return err
		}
		out.WriteString(text)
	case *ast.Paragraph:
		text, err := renderInlineChildren(n, source, vars)
		if err != nil {
			return err
		}
		out.WriteString(text)
	case *ast.Blockquote:
		var buf strings.Builder
		if err := renderBlockChildren(&buf, n, source, vars); err != nil {
			return err
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		for i, line := range lines {
			if i > 0 {
				out.WriteByte('\n')
			}
			out.WriteString("> ")
			out.WriteString(strings.TrimRight(line, "\r"))
		}
	case *ast.List:
		return renderList(out, n, source, vars)
	case *ast.FencedCodeBlock:
		renderFencedCodeBlock(out, n, source)
	case *ast.CodeBlock:
		renderIndentedCodeBlock(out, n, source)
	case *ast.HTMLBlock:
		out.Write(n.Text(source))
	case *ast.ThematicBreak:
		out.WriteString("---")
	default:
		if node.HasChildren() {
			return renderBlockChildren(out, node, source, vars)
		}
	}
	return nil
}

func renderList(out *strings.Builder, list *ast.List, source []byte, vars map[string]string) error {
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		if item != list.FirstChild() {
			out.WriteByte('\n')
		}
		listItem, ok := item.(*ast.ListItem)
		if !ok {
			continue
		}
		text, err := renderListItem(listItem, source, vars)
		if err != nil {
			return err
		}
		out.WriteString(text)
	}
	return nil
}

func renderListItem(item *ast.ListItem, source []byte, vars map[string]string) (string, error) {
	var out strings.Builder
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Paragraph:
			text, err := renderInlineChildren(n, source, vars)
			if err != nil {
				return "", err
			}
			if out.Len() == 0 {
				out.WriteString("- ")
				out.WriteString(text)
			} else {
				out.WriteByte('\n')
				out.WriteString(text)
			}
		case *ast.Blockquote, *ast.List, *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock:
			var block strings.Builder
			if err := renderBlockNode(&block, n, source, vars); err != nil {
				return "", err
			}
			if out.Len() == 0 {
				out.WriteString("- ")
			} else {
				out.WriteByte('\n')
			}
			out.WriteString(block.String())
		default:
			if n.HasChildren() {
				text, err := renderInlineChildren(n, source, vars)
				if err != nil {
					return "", err
				}
				if out.Len() == 0 {
					out.WriteString("- ")
					out.WriteString(text)
				} else {
					out.WriteByte('\n')
					out.WriteString(text)
				}
			}
		}
	}
	return out.String(), nil
}

func renderFencedCodeBlock(out *strings.Builder, n *ast.FencedCodeBlock, source []byte) {
	if lang := strings.TrimSpace(string(n.Language(source))); lang != "" {
		out.WriteString(Str17460_markdown)
		out.WriteString(lang)
		out.WriteByte('\n')
	} else {
		out.WriteString("```\n")
	}
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		out.Write(seg.Value(source))
	}
	if lines.Len() == 0 {
		out.WriteByte('\n')
	} else {
		last := lines.At(lines.Len() - 1)
		if !bytes.HasSuffix(last.Value(source), []byte("\n")) {
			out.WriteByte('\n')
		}
	}
	out.WriteString(Str17460_markdown)
}

func renderIndentedCodeBlock(out *strings.Builder, n *ast.CodeBlock, source []byte) {
	out.WriteString("```\n")
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		out.Write(seg.Value(source))
	}
	if lines.Len() == 0 {
		out.WriteByte('\n')
	} else {
		last := lines.At(lines.Len() - 1)
		if !bytes.HasSuffix(last.Value(source), []byte("\n")) {
			out.WriteByte('\n')
		}
	}
	out.WriteString(Str17460_markdown)
}

func renderInlineChildren(parent ast.Node, source []byte, vars map[string]string) (string, error) {
	var out strings.Builder
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		if err := renderInlineNode(&out, child, source, vars); err != nil {
			return "", err
		}
	}
	return out.String(), nil
}

func renderInlineNode(out *strings.Builder, node ast.Node, source []byte, vars map[string]string) error {
	switch n := node.(type) {
	case *ast.Text:
		seg := n.Segment
		if n.IsRaw() {
			out.Write(seg.Value(source))
			if n.SoftLineBreak() {
				out.WriteByte('\n')
			}
			return nil
		}
		value, err := substituteMarkdownText(string(seg.Value(source)), vars)
		if err != nil {
			return err
		}
		out.WriteString(value)
		if n.SoftLineBreak() {
			out.WriteByte('\n')
		}
	case *ast.CodeSpan:
		out.WriteByte('`')
		out.WriteString(string(n.Text(source)))
		out.WriteByte('`')
	case *ast.RawHTML:
		for i := 0; i < n.Segments.Len(); i++ {
			seg := n.Segments.At(i)
			out.Write(seg.Value(source))
		}
	case *ast.Emphasis:
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			if err := renderInlineNode(out, child, source, vars); err != nil {
				return err
			}
		}
	case *ast.Link:
		out.WriteByte('[')
		label, err := renderInlineChildren(n, source, vars)
		if err != nil {
			return err
		}
		out.WriteString(label)
		out.WriteByte(']')
		out.WriteByte('(')
		out.WriteString(string(n.Destination))
		if len(n.Title) > 0 {
			out.WriteByte(' ')
			out.WriteByte('"')
			out.WriteString(string(n.Title))
			out.WriteByte('"')
		}
		out.WriteByte(')')
	case *ast.Image:
		out.WriteString("![")
		alt, err := renderInlineChildren(n, source, vars)
		if err != nil {
			return err
		}
		out.WriteString(alt)
		out.WriteByte(']')
		out.WriteByte('(')
		out.WriteString(string(n.Destination))
		if len(n.Title) > 0 {
			out.WriteByte(' ')
			out.WriteByte('"')
			out.WriteString(string(n.Title))
			out.WriteByte('"')
		}
		out.WriteByte(')')
	case *ast.AutoLink:
		out.WriteByte('<')
		out.Write(n.URL(source))
		out.WriteByte('>')
	default:
		if node.HasChildren() {
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				if err := renderInlineNode(out, child, source, vars); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func substituteMarkdownText(text string, vars map[string]string) (string, error) {
	if text == "" {
		return "", nil
	}

	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if ch == '\\' && i+1 < len(text) {
			switch text[i+1] {
			case '{', '}':
				out.WriteByte(text[i+1])
				i++
				continue
			}
		}
		if ch != '{' {
			out.WriteByte(ch)
			continue
		}

		j := i + 1
		for j < len(text) && text[j] != '}' && text[j] != '\n' {
			j++
		}
		if j >= len(text) || text[j] != '}' {
			out.WriteByte(ch)
			continue
		}

		name := text[i+1 : j]
		if !validIdentifier(name) {
			return "", &InvalidVariableReferenceError{Reference: name}
		}
		value, ok := vars[name]
		if !ok {
			return "", &UnknownVariableError{Name: name}
		}
		out.WriteString(value)
		i = j
	}
	return out.String(), nil
}

func markdownReferencedVariables(body string) map[string]bool {
	source := []byte(body)
	doc := markdownParser.Parser().Parse(gmtext.NewReader(source))

	used := make(map[string]bool)
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.Text:
			seg := n.Segment
			collectMarkdownVariables(string(seg.Value(source)), func(name string) {
				used[name] = true
			})
		case *ast.CodeSpan, *ast.RawHTML, *ast.FencedCodeBlock, *ast.CodeBlock:
			return ast.WalkSkipChildren, nil
		default:
			_ = n
		}
		return ast.WalkContinue, nil
	})
	return used
}

func collectMarkdownVariables(text string, visit func(name string)) {
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if ch == '\\' && i+1 < len(text) {
			switch text[i+1] {
			case '{', '}':
				i++
				continue
			}
		}
		if ch != '{' {
			continue
		}
		j := i + 1
		for j < len(text) && text[j] != '}' && text[j] != '\n' {
			j++
		}
		if j >= len(text) || text[j] != '}' {
			continue
		}
		name := text[i+1 : j]
		if validIdentifier(name) {
			visit(name)
		}
		i = j
	}
}
