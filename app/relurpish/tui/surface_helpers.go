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

// ApplyInteractionFrame lets the active chat pane update any surface-local
// sidebar or frame state when it supports the interaction protocol.
func (m *RootModel) ApplyInteractionFrame(frame interaction.InteractionFrame) {
	if m == nil || m.chat == nil {
		return
	}
	if updater, ok := m.chat.(interface {
		UpdateSidebarFromFrame(interaction.InteractionFrame)
	}); ok {
		updater.UpdateSidebarFromFrame(frame)
	}
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
