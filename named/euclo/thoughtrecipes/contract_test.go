package thoughtrecipe

import "testing"

func TestThoughtRecipeContractRootAndIdentity(t *testing.T) {
	if ThoughtRecipeSourceRoot != "relurpify_cfg/euclo" {
		t.Fatalf("ThoughtRecipeSourceRoot = %q, want %q", ThoughtRecipeSourceRoot, "relurpify_cfg/euclo")
	}
	if ThoughtRecipeIdentityHeader != "thoughtrecipe" {
		t.Fatalf("ThoughtRecipeIdentityHeader = %q, want %q", ThoughtRecipeIdentityHeader, "thoughtrecipe")
	}
}

func TestThoughtRecipeContractAcceptedExtensions(t *testing.T) {
	want := []string{".erpe", ".euclo", ".thoughtrecipe"}
	if len(AcceptedThoughtRecipeExtensions) != len(want) {
		t.Fatalf("AcceptedThoughtRecipeExtensions = %v, want %v", AcceptedThoughtRecipeExtensions, want)
	}
	for i, ext := range want {
		if AcceptedThoughtRecipeExtensions[i] != ext {
			t.Fatalf("AcceptedThoughtRecipeExtensions[%d] = %q, want %q", i, AcceptedThoughtRecipeExtensions[i], ext)
		}
		if !IsAcceptedThoughtRecipeExtension(ext) {
			t.Fatalf("expected %q to be accepted", ext)
		}
	}
	if IsAcceptedThoughtRecipeExtension(".yaml") {
		t.Fatal("did not expect .yaml to be accepted")
	}
}

func TestThoughtRecipeContractSurfaceFreeze(t *testing.T) {
	wantDeclarations := []string{"thoughtrecipe", "trigger", "input", "type", "agent", "run", "route", "delegate", "ask", "pipeline"}
	if len(AllowedTopLevelDeclarations) != len(wantDeclarations) {
		t.Fatalf("AllowedTopLevelDeclarations = %v, want %v", AllowedTopLevelDeclarations, wantDeclarations)
	}
	for i, decl := range wantDeclarations {
		if AllowedTopLevelDeclarations[i] != decl {
			t.Fatalf("AllowedTopLevelDeclarations[%d] = %q, want %q", i, AllowedTopLevelDeclarations[i], decl)
		}
	}

	wantNamespaces := []string{"input.*", "state.*", "scratch.*", "user.*", "output.*"}
	if len(SupportedNamespaces) != len(wantNamespaces) {
		t.Fatalf("SupportedNamespaces = %v, want %v", SupportedNamespaces, wantNamespaces)
	}
	for i, ns := range wantNamespaces {
		if SupportedNamespaces[i] != ns {
			t.Fatalf("SupportedNamespaces[%d] = %q, want %q", i, SupportedNamespaces[i], ns)
		}
	}
}
