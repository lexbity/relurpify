package python

/*
func TestVerificationResolver_BuildPlan(t *testing.T) {
	resolver := NewVerificationResolver()
	plan, ok, err := resolver.BuildPlan(context.Background(), contracts.VerificationPlanRequest{
		TaskInstruction: "verify this Python change",
		Workspace:       ".",
		Files:           []string{"app/service.py"},
		TestFiles:       []string{"tests/test_service.py"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ok {
		t.Fatal("expected plan")
	}
	if plan.ScopeKind != "test_files" {
		t.Fatalf("expected test_files, got %q", plan.ScopeKind)
	}
	if len(plan.Commands) != 1 || plan.Commands[0].Command[0] != "python" {
		t.Fatalf("expected python pytest command, got %#v", plan.Commands)
	}
}
*/
