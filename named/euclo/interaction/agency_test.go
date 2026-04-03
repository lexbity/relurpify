package interaction

import (
	"testing"
)

func TestNewAgencyResolver(t *testing.T) {
	resolver := NewAgencyResolver()
	if resolver == nil {
		t.Fatal("NewAgencyResolver returned nil")
	}
}

func TestRegisterTrigger(t *testing.T) {
	resolver := NewAgencyResolver()
	
	trigger := AgencyTrigger{
		Phrases:      []string{"yes", "ok", "confirm"},
		CapabilityID: "test.capability",
		Description:  "Test trigger",
	}
	
	resolver.RegisterTrigger("chat", trigger)
	
	// Verify trigger was registered
	triggers := resolver.TriggersForMode("chat")
	if len(triggers) != 1 {
		t.Fatalf("Expected 1 trigger for chat mode, got %d", len(triggers))
	}
	if triggers[0].CapabilityID != trigger.CapabilityID {
		t.Errorf("Expected CapabilityID %s, got %s", trigger.CapabilityID, triggers[0].CapabilityID)
	}
}

func TestResolveExactMatch(t *testing.T) {
	resolver := NewAgencyResolver()
	
	trigger := AgencyTrigger{
		Phrases:      []string{"implement this", "write code"},
		CapabilityID: "code.implement",
		RequiresMode: "chat",
	}
	resolver.RegisterTrigger("chat", trigger)
	
	// Exact match
	found, ok := resolver.Resolve("chat", "implement this")
	if !ok {
		t.Error("Resolve should find exact match")
	}
	if found == nil || found.CapabilityID != trigger.CapabilityID {
		t.Error("Resolve returned wrong trigger")
	}
	
	// Case insensitive
	found, ok = resolver.Resolve("chat", "IMPLEMENT THIS")
	if !ok {
		t.Error("Resolve should be case insensitive")
	}
	
	// Different mode
	found, ok = resolver.Resolve("debug", "implement this")
	if ok {
		t.Error("Resolve should not match in different mode")
	}
}

func TestResolveFuzzyMatch(t *testing.T) {
	resolver := NewAgencyResolver()
	
	trigger := AgencyTrigger{
		Phrases:      []string{"implement"},
		CapabilityID: "code.implement",
	}
	resolver.RegisterTrigger("chat", trigger)
	
	// Fuzzy match (contains phrase)
	found, ok := resolver.Resolve("chat", "please implement this feature")
	if !ok {
		t.Error("Resolve should find fuzzy match")
	}
	if found == nil {
		t.Error("Resolve returned nil for fuzzy match")
	}
	
	// No match
	found, ok = resolver.Resolve("chat", "write some code")
	if ok {
		t.Error("Resolve should not match unrelated text")
	}
}

func TestResolveEmptyText(t *testing.T) {
	resolver := NewAgencyResolver()
	
	trigger := AgencyTrigger{
		Phrases: []string{"test"},
	}
	resolver.RegisterTrigger("chat", trigger)
	
	// Empty text
	found, ok := resolver.Resolve("chat", "")
	if ok {
		t.Error("Resolve should not match empty text")
	}
	if found != nil {
		t.Error("Resolve should return nil for empty text")
	}
	
	// Whitespace only
	found, ok = resolver.Resolve("chat", "   ")
	if ok {
		t.Error("Resolve should not match whitespace-only text")
	}
}

func TestTriggersForMode(t *testing.T) {
	resolver := NewAgencyResolver()
	
	// Register triggers for different modes
	chatTrigger := AgencyTrigger{
		Phrases:      []string{"chat"},
		CapabilityID: "chat.trigger",
	}
	debugTrigger := AgencyTrigger{
		Phrases:      []string{"debug"},
		CapabilityID: "debug.trigger",
	}
	globalTrigger := AgencyTrigger{
		Phrases:      []string{"global"},
		CapabilityID: "global.trigger",
	}
	
	resolver.RegisterTrigger("chat", chatTrigger)
	resolver.RegisterTrigger("debug", debugTrigger)
	resolver.RegisterTrigger("", globalTrigger) // Global trigger
	
	// Test chat mode
	triggers := resolver.TriggersForMode("chat")
	if len(triggers) != 2 { // chatTrigger + globalTrigger
		t.Errorf("Expected 2 triggers for chat mode, got %d", len(triggers))
	}
	
	// Test debug mode
	triggers = resolver.TriggersForMode("debug")
	if len(triggers) != 2 { // debugTrigger + globalTrigger
		t.Errorf("Expected 2 triggers for debug mode, got %d", len(triggers))
	}
	
	// Test unknown mode
	triggers = resolver.TriggersForMode("unknown")
	if len(triggers) != 1 { // Only globalTrigger
		t.Errorf("Expected 1 trigger for unknown mode, got %d", len(triggers))
	}
}
