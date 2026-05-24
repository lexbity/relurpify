package tui

import (
	"context"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// PushNotification enqueues a notification for the host notification bar.
func (m *RootModel) PushNotification(item NotificationItem) {
	if m == nil || m.notifQ == nil || item.Kind == "" {
		return
	}
	m.notifQ.Push(item)
}

// AppendSurfaceMessage appends a rendered message to the active chat surface.
func (m *RootModel) AppendSurfaceMessage(msg Message) {
	if m == nil || m.chat == nil {
		return
	}
	m.chat.AppendMessage(msg)
}

// ApplyInteractionFrame lets the active chat pane update surface-local sidebar
// or frame state when it supports the interaction protocol.
func (m *RootModel) ApplyInteractionFrame(frame interaction.InteractionFrame) {
	if m == nil || m.chat == nil {
		return
	}
	if updater, ok := m.chat.(interface {
		UpdateSidebarFromFrame(any)
	}); ok {
		updater.UpdateSidebarFromFrame(frame)
	}
}

// TrackInteractionFrame records a pending interaction frame so the host can
// resolve it later from the notification or guidance surfaces.
func (m *RootModel) TrackInteractionFrame(notificationID string, frame interaction.InteractionFrame) {
	m.trackInteractionFrame(notificationID, frame)
}

// OpenInteractionGuidance opens the guidance panel for a freetext interaction frame.
func (m *RootModel) OpenInteractionGuidance(notificationID string, frame interaction.InteractionFrame) {
	m.openInteractionGuidance(notificationID, frame)
}

// HandleSurfaceFrame is a convenience helper for surfaces that want to apply a
// rendered frame without duplicating the host-side bookkeeping.
func (m *RootModel) HandleSurfaceFrame(_ context.Context, msg SurfaceFrameMsg) {
	if m == nil {
		return
	}
	m.PushNotification(msg.Notification)
	m.AppendSurfaceMessage(msg.Message)
}
