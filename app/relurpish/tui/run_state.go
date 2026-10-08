package tui

// UpdateTaskMsg allows external messages to update plan task status in-place.
type UpdateTaskMsg struct {
	TaskIndex int
	Status    TaskStatus
}
