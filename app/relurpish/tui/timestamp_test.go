package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFormatMessageTimestampToday verifies today's timestamps show only time.
func TestFormatMessageTimestampToday(t *testing.T) {
	// Create a timestamp from today
	now := time.Now()
	ts := time.Date(now.Year(), now.Month(), now.Day(), 14, 35, 0, 0, time.Local)

	result := formatMessageTimestamp(ts)

	// Should be in HH:MM format
	require.Regexp(t, `^\d{2}:\d{2}$`, result)
	require.Equal(t, "14:35", result)
}

// TestFormatMessageTimestampYesterday verifies past dates show date and time.
func TestFormatMessageTimestampYesterday(t *testing.T) {
	// Create a timestamp from yesterday
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	ts := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 14, 35, 0, 0, time.Local)

	result := formatMessageTimestamp(ts)

	// Should be in "Mon 02 HH:MM" format
	require.Regexp(t, `^[A-Z][a-z]{2}\s+\d{2}\s+\d{2}:\d{2}$`, result)
}

// TestFormatMessageTimestampLastWeek verifies older timestamps are formatted correctly.
func TestFormatMessageTimestampLastWeek(t *testing.T) {
	// Create a timestamp from last week
	now := time.Now()
	lastWeek := now.AddDate(0, 0, -7)
	ts := time.Date(lastWeek.Year(), lastWeek.Month(), lastWeek.Day(), 9, 15, 0, 0, time.Local)

	result := formatMessageTimestamp(ts)

	// Should be in "Mon 02 HH:MM" format
	require.Regexp(t, `^[A-Z][a-z]{2}\s+\d{2}\s+\d{2}:\d{2}$`, result)
	// Should contain 09:15
	require.Contains(t, result, "09:15")
}

// TestFormatMessageTimestampLastMonth verifies old timestamps are formatted correctly.
func TestFormatMessageTimestampLastMonth(t *testing.T) {
	// Create a timestamp from last month
	now := time.Now()
	lastMonth := now.AddDate(0, -1, 0)
	ts := time.Date(lastMonth.Year(), lastMonth.Month(), lastMonth.Day(), 10, 20, 0, 0, time.Local)

	result := formatMessageTimestamp(ts)

	// Should include time
	require.Regexp(t, `\d{2}:\d{2}$`, result)
}

// TestRenderMessageHeaderIncludesTimestamp verifies headers include formatted timestamps.
func TestRenderMessageHeaderIncludesTimestamp(t *testing.T) {
	now := time.Now()
	msg := Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Timestamp: time.Date(now.Year(), now.Month(), now.Day(), 14, 35, 0, 0, time.Local),
		Content:   MessageContent{Text: "Hello"},
	}

	header := renderMsgHeader(msg)

	// Should contain the formatted timestamp
	require.Contains(t, header, "14:35")
	// Should contain the role
	require.Contains(t, header, "You")
	// Should contain the icon
	require.Contains(t, header, "👤")
}

// TestRenderMessageHeaderAgentWithTimestamp verifies agent messages include timestamps.
func TestRenderMessageHeaderAgentWithTimestamp(t *testing.T) {
	now := time.Now()
	msg := Message{
		ID:        "msg-1",
		Role:      RoleAgent,
		Timestamp: time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, time.Local),
		Content:   MessageContent{Text: "Response"},
	}

	header := renderMsgHeader(msg)

	// Should contain the time
	require.Contains(t, header, "10:00")
	// Should contain agent role
	require.Contains(t, header, "Agent")
	// Should contain agent icon
	require.Contains(t, header, "🤖")
}

// TestRenderMessageHeaderSystemWithTimestamp verifies system messages include timestamps.
func TestRenderMessageHeaderSystemWithTimestamp(t *testing.T) {
	now := time.Now()
	msg := Message{
		ID:        "msg-1",
		Role:      RoleSystem,
		Timestamp: time.Date(now.Year(), now.Month(), now.Day(), 12, 30, 0, 0, time.Local),
		Content:   MessageContent{Text: "Info"},
	}

	header := renderMsgHeader(msg)

	// Should contain the time
	require.Contains(t, header, "12:30")
	// Should contain system role
	require.Contains(t, header, "System")
	// Should contain system icon
	require.Contains(t, header, "⚙")
}

// TestRenderMessageIncludesTimestampInFullOutput verifies timestamps appear in rendered messages.
func TestRenderMessageIncludesTimestampInFullOutput(t *testing.T) {
	now := time.Now()
	msg := Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Timestamp: time.Date(now.Year(), now.Month(), now.Day(), 15, 45, 0, 0, time.Local),
		Content:   MessageContent{Text: "Test message"},
	}

	rendered := RenderMessage(msg, 80, "")

	// Should include the formatted timestamp
	require.Contains(t, rendered, "15:45")
	// Should include the message content
	require.Contains(t, rendered, "Test message")
}

// TestTimestampFormatBoundary verifies midnight timestamp is handled correctly.
func TestTimestampFormatBoundary(t *testing.T) {
	now := time.Now()
	// Midnight today
	ts := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	result := formatMessageTimestamp(ts)

	// Should be today's format (HH:MM)
	require.Equal(t, "00:00", result)
}

// TestTimestampFormatCrossDayBoundary verifies one second before midnight shows yesterday's date.
func TestTimestampFormatCrossDayBoundary(t *testing.T) {
	now := time.Now()
	// One second before midnight
	yesterday := now.AddDate(0, 0, -1)
	ts := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 0, time.Local)

	result := formatMessageTimestamp(ts)

	// Should include date since it's not today
	require.Regexp(t, `^[A-Z][a-z]{2}\s+\d{2}\s+\d{2}:\d{2}$`, result)
	// Should show 23:59
	require.Contains(t, result, "23:59")
}
