package sandbox

import "codeburg.org/lexbit/relurpify/platform/contracts"

// IsPrivateOrLoopbackHost is re-exported from platform/contracts so the
// framework/sandbox surface is preserved for existing callers (gVisor policy
// validation, framework/authorization network checks). The canonical denylist
// implementation lives in platform/contracts — the sandbox contract leaf — so
// platform-level network tools can enforce the same boundary without importing
// framework.
var IsPrivateOrLoopbackHost = contracts.IsPrivateOrLoopbackHost
