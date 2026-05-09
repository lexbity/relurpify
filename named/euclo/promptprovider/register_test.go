package promptprovider

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/prompt"
)

func TestRegisterAllIsIdempotent(t *testing.T) {
	registry := prompt.NewRegistry()
	if err := RegisterAll(registry); err != nil {
		t.Fatalf("first RegisterAll failed: %v", err)
	}
	if err := RegisterAll(registry); err != nil {
		t.Fatalf("second RegisterAll failed: %v", err)
	}
}
