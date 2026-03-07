package browser

import (
	"context"
	"encoding/json"
	"time"
)

// Backend defines the transport-agnostic browser automation contract.
type Backend interface {
	Navigate(ctx context.Context, url string) error
	Click(ctx context.Context, selector string) error
	Type(ctx context.Context, selector, text string) error
	GetText(ctx context.Context, selector string) (string, error)
	GetAccessibilityTree(ctx context.Context) (string, error)
	GetHTML(ctx context.Context) (string, error)
	ExecuteScript(ctx context.Context, script string) (any, error)
	Screenshot(ctx context.Context) ([]byte, error)
	WaitFor(ctx context.Context, condition WaitCondition, timeout time.Duration) error
	CurrentURL(ctx context.Context) (string, error)
	Close() error
}

// BrowserBackend preserves the more explicit external name used by design docs.
type BrowserBackend = Backend

// Capabilities describe which higher-level browser features a backend can
// reliably support.
type Capabilities struct {
	AccessibilityTree bool `json:"accessibility_tree"`
	NetworkIntercept  bool `json:"network_intercept"`
	DownloadEvents    bool `json:"download_events"`
	PopupTracking     bool `json:"popup_tracking"`
	ArbitraryEval     bool `json:"arbitrary_eval"`
}

// CapabilityReporter allows a backend to advertise transport-specific support.
type CapabilityReporter interface {
	Capabilities() Capabilities
}

// StructuredPageData captures compact semantically useful page information.
type StructuredPageData struct {
	URL      string            `json:"url"`
	Title    string            `json:"title"`
	Headings []string          `json:"headings,omitempty"`
	Links    []StructuredLink  `json:"links,omitempty"`
	Inputs   []StructuredInput `json:"inputs,omitempty"`
	Buttons  []string          `json:"buttons,omitempty"`
	Code     []string          `json:"code,omitempty"`
}

type StructuredLink struct {
	Text string `json:"text"`
	Href string `json:"href"`
}

type StructuredInput struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Placeholder string `json:"placeholder"`
}

// MarshalJSON is a small helper so callers can use the same deterministic
// representation for context budgeting and tool results.
func (s StructuredPageData) MarshalJSON() ([]byte, error) {
	type alias StructuredPageData
	return json.Marshal(alias(s))
}

// WaitConditionType identifies the event or state the backend should wait for.
type WaitConditionType string

const (
	WaitForLoad            WaitConditionType = "load"
	WaitForNetworkIdle     WaitConditionType = "network_idle"
	WaitForSelector        WaitConditionType = "selector"
	WaitForSelectorMissing WaitConditionType = "selector_missing"
	WaitForText            WaitConditionType = "text"
	WaitForURLContains     WaitConditionType = "url_contains"
)

// WaitCondition describes a wait target in a transport-neutral form.
type WaitCondition struct {
	Type        WaitConditionType
	Selector    string
	Text        string
	URLContains string
}
