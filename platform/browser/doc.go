// Package browser provides backend-agnostic browser automation tools for
// agents that need to interact with web interfaces as part of their tasks.
//
// session.go exposes the BrowserSession wrapper used by the browser service
// layer to apply navigation policy, extraction budgeting, page-state capture,
// and error normalization around a raw backend.
//
// The backend subpackages provide transport-specific implementations:
//
//   - cdp: Chrome DevTools Protocol over websocket
//   - bidi: WebDriver session startup plus BiDi transport
//   - webdriver: classic WebDriver session transport
//
// All browser launches still pass through sandbox command policy, and browser
// navigation still consults the permission manager for network checks.
package browser
