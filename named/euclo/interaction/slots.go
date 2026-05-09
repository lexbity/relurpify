package interaction

// DefaultAction returns the default action for a frame.
func DefaultAction(frame *InteractionFrame) string {
	return frame.DefaultSlot
}

// LookupSlot finds an action slot by ID.
func LookupSlot(frame *InteractionFrame, slotID string) (*ActionSlot, bool) {
	for i := range frame.Slots {
		if frame.Slots[i].ID == slotID {
			return &frame.Slots[i], true
		}
	}
	return nil, false
}

// SlotChoice represents a user's slot selection with optional extra data.
type SlotChoice struct {
	SlotID    string
	ExtraData map[string]any
}
