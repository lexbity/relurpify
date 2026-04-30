package euclotui

import (
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// NewFrameMsg packages an interaction frame into the generic surface message
// format used by the relurpish host.
func NewFrameMsg(frame interaction.InteractionFrame) tui.SurfaceFrameMsg {
	return tui.SurfaceFrameMsg{
		Surface:      "euclo",
		Message:      RenderInteractionFrame(frame),
		Frame:        frame,
		Notification: notificationItemFromFrame(tui.GenerateID(), NotifKindInteraction, frame, nil),
	}
}
