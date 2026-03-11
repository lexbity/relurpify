// Package browser provides Selenium/WebDriver-based browser automation tools
// for agents that need to interact with web interfaces as part of their tasks.
//
// backend.go implements the WebDriver backend, managing browser process
// lifecycle and the WebDriver HTTP session. session.go exposes a BrowserSession
// to agents with tools for navigation, element interaction, screenshot capture,
// and JavaScript execution. All browser operations run through the sandbox
// runner and are gated by the agent's network permission declarations.
package browser
