// Package channel provides the adapter layer for concurrent inbound and
// outbound agent communication pipelines.
//
// # Adapter
//
// Adapter normalizes inbound messages from an upstream service (such as a chat
// relay, Discord, or Telegram) into framework events and delivers outbound
// replies. Each adapter has a lifecycle (Start/Stop), a Send method for
// outbound messages, and a Status report. Adapters emit normalized events via
// EventSink.
//
// # Manager
//
// Manager supervises a collection of registered adapters. It starts and stops
// all adapters together, routes outbound messages to the correct adapter by
// channel name, and exposes a per-adapter Status map. Adapters are registered
// before Start is called and can be restarted individually via Restart.
//
// # Message types
//
// InboundMessage carries a normalized sender Identity, Conversation context,
// and MessageContent (text, media, reactions, structured payloads).
// OutboundMessage carries the channel name and conversation ID needed to route
// a reply back to the originating conversation.
package channel
