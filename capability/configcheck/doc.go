// Package configcheck validates tool manifests against expected capability
// declarations. It derives the expected risk_class and effect_class from the
// tool's binary command and sandbox configuration, then hard-fails when the
// manifest under-declares its capability footprint.
//
// This is the SEC-2 enforcement point: every tool manifest must accurately
// declare its effect on the system. Silent under-declaration (e.g. a network
// tool claiming only process_spawn) is caught at validation time.
// Package configcheck validates tool manifests against expected capability
// declarations. configcheck is product workspace-linter logic owned by the
// capability domain: it judges manifests over manifest data the domain
// itself defines (relurplint consumes it; it is testable independently of
// any binary).
package configcheck
