package core

// Provenance is the unified provenance record for any content in the system.
// It combines origin provenance (capability boundary), derivation (transformation boundary),
// and anchors (semantic boundary) into a single queryable record.
type Provenance struct {
	Origin       *ContentProvenance `json:"origin,omitempty"`
	Derivation   *DerivationChain   `json:"derivation,omitempty"`
	Anchors      []AnchorRef        `json:"anchors,omitempty"`
	AnchorStatus map[string]string  `json:"anchor_status,omitempty"` // anchor_id → "fresh" | "drifted" | "superseded"
}

// ContentWithProvenance is an optional interface for any type that carries unified provenance.
// Types implementing this interface expose their full provenance for inspection and tracking.
type ContentWithProvenance interface {
	ContentProvenance() *Provenance
}
