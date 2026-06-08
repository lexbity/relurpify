package ports

import "testing"

// fakeState implements State for testing.
type fakeState struct {
	data    map[string]any
	taskID  string
	sessID  string
}

func (s *fakeState) GetWorkingValue(key string) (any, bool) {
	v, ok := s.data[key]
	return v, ok
}

func (s *fakeState) SetWorkingValue(key string, value any) {
	if s.data == nil {
		s.data = make(map[string]any)
	}
	s.data[key] = value
}

func (s *fakeState) DeleteWorkingValue(key string) {
	delete(s.data, key)
}

func (s *fakeState) ClearWorkingData() {
	s.data = make(map[string]any)
}

func (s *fakeState) WorkingMemoryKeys() []string {
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}

func (s *fakeState) Snapshot() map[string]any {
	out := make(map[string]any, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}

func (s *fakeState) TaskID() string    { return s.taskID }
func (s *fakeState) SessionID() string { return s.sessID }

func TestStateInterface(t *testing.T) {
	var _ State = (*fakeState)(nil)
}

func TestStateSetGet(t *testing.T) {
	s := &fakeState{taskID: "task-1", sessID: "sess-1"}

	s.SetWorkingValue("key1", "value1")
	v, ok := s.GetWorkingValue("key1")
	if !ok {
		t.Fatal("expected key1 to exist")
	}
	if v != "value1" {
		t.Errorf("got %v, want value1", v)
	}
}

func TestStateDelete(t *testing.T) {
	s := &fakeState{data: map[string]any{"k": "v"}}
	s.DeleteWorkingValue("k")
	_, ok := s.GetWorkingValue("k")
	if ok {
		t.Error("expected k to be deleted")
	}
}

func TestStateClear(t *testing.T) {
	s := &fakeState{data: map[string]any{"a": 1, "b": 2}}
	s.ClearWorkingData()
	if len(s.WorkingMemoryKeys()) != 0 {
		t.Error("expected empty after clear")
	}
}

func TestStateSnapshot(t *testing.T) {
	s := &fakeState{data: map[string]any{"x": "y"}}
	snap := s.Snapshot()
	if snap["x"] != "y" {
		t.Errorf("snapshot missing value, got %v", snap)
	}
	// Mutating snapshot should not affect original
	snap["x"] = "z"
	orig, _ := s.GetWorkingValue("x")
	if orig != "y" {
		t.Error("snapshot mutation affected original")
	}
}

func TestStateMetadata(t *testing.T) {
	s := &fakeState{taskID: "task-1", sessID: "sess-1"}
	if s.TaskID() != "task-1" {
		t.Errorf("TaskID() = %q, want %q", s.TaskID(), "task-1")
	}
	if s.SessionID() != "sess-1" {
		t.Errorf("SessionID() = %q, want %q", s.SessionID(), "sess-1")
	}
}
