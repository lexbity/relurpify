package core

// SensitivityClass categorizes the sensitivity level of data or operations.
type SensitivityClass string

const (
	// SensitivityClassLow indicates low sensitivity data/operations.
	SensitivityClassLow SensitivityClass = "low"
	// SensitivityClassMedium indicates medium sensitivity data/operations.
	SensitivityClassMedium SensitivityClass = "medium"
	// SensitivityClassHigh indicates high sensitivity data/operations.
	SensitivityClassHigh SensitivityClass = "high"
	// SensitivityClassCritical indicates critical sensitivity data/operations.
	SensitivityClassCritical SensitivityClass = "critical"
)

// RouteMode defines how a request should be routed.
type RouteMode string

const (
	// RouteModeDirect indicates direct routing without gateway.
	RouteModeDirect RouteMode = "direct"
	// RouteModeGateway indicates routing through a gateway.
	RouteModeGateway RouteMode = "gateway"
	// RouteModeProxy indicates routing through a proxy.
	RouteModeProxy RouteMode = "proxy"
)
