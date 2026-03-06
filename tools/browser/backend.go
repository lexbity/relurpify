package browser

import (
	"context"
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
