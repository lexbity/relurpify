package chainer

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

func TestChainerContractRegistered(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("chainer")
	if !ok || c == nil {
		t.Fatal("expected chainer paradigm contract to be registered")
	}
	if c.Shape != paradigm.ShapeChain {
		t.Fatalf("chainer shape = %q, want %q", c.Shape, paradigm.ShapeChain)
	}
	link, ok := c.Directive("link")
	if !ok {
		t.Fatal("expected chainer `link` directive")
	}
	if link.Form != paradigm.FormBlock {
		t.Fatalf("chainer `link` form = %q, want block", link.Form)
	}
	if !link.Required || !link.Repeatable {
		t.Fatalf("chainer `link` spec = %#v, want required+repeatable", link)
	}
	allowed := map[paradigm.BodyItem]bool{}
	for _, item := range link.Body {
		allowed[item] = true
	}
	for _, want := range []paradigm.BodyItem{paradigm.BodyItemFrom, paradigm.BodyItemCapture, paradigm.BodyItemDirective} {
		if !allowed[want] {
			t.Errorf("chainer `link` body missing allowed item %q", want)
		}
	}
	for _, forbidden := range []paradigm.BodyItem{paradigm.BodyItemDo, paradigm.BodyItemRun, paradigm.BodyItemDelegate} {
		if allowed[forbidden] {
			t.Errorf("chainer `link` body must not allow %q", forbidden)
		}
	}
}

func TestContractCompleteness(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("chainer")
	if !ok || c == nil {
		t.Fatal("expected chainer paradigm contract to be registered")
	}
	declared := map[string]bool{}
	for _, spec := range c.Directives {
		declared[spec.Name] = true
	}
	pinned := map[string]bool{}
	for _, cas := range c.Conformance {
		if strings.TrimSpace(cas.ID) == "" {
			t.Errorf("conformance case with empty ID")
		}
		if strings.TrimSpace(cas.Directive) == "" {
			continue
		}
		if !declared[cas.Directive] {
			t.Errorf("case %q pins undeclared directive %q", cas.ID, cas.Directive)
			continue
		}
		pinned[cas.Directive] = true
	}
	for _, spec := range c.Directives {
		if !pinned[spec.Name] {
			t.Errorf("directive %q has no conformance case", spec.Name)
		}
	}
	for _, composed := range c.Composes {
		if _, ok := paradigm.Registry.Lookup(composed); !ok {
			t.Errorf("chainer composes %q, which has no registered contract", composed)
		}
	}
}
