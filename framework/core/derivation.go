package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// DerivationID is a content-addressed identifier for a derivation step.
// Computed as: sha256(parent_id + ":" + transform + ":" + timestamp_nanos)[:16] hex-encoded.
type DerivationID string

// DerivationStep records a single transformation in a content's lifecycle.
type DerivationStep struct {
	ID            DerivationID `json:"id"`
	ParentID      DerivationID `json:"parent_id,omitempty"`
	Transform     string       `json:"transform"`
	LossMagnitude float64      `json:"loss_magnitude"`
	SourceSystem  string       `json:"source_system"`
	Timestamp     time.Time    `json:"timestamp"`
	Detail        string       `json:"detail,omitempty"`
}

// DerivationChain is an ordered sequence of derivation steps from origin to current.
type DerivationChain struct {
	Steps []DerivationStep `json:"steps,omitempty"`
}

// generateDerivationID generates a content-addressed ID for a derivation step.
func generateDerivationID(parentID DerivationID, transform string, timestamp time.Time) DerivationID {
	hash := sha256.New()
	hash.Write([]byte(string(parentID) + ":" + transform + ":" + fmt.Sprintf("%d", timestamp.UnixNano())))
	hexStr := hex.EncodeToString(hash.Sum(nil))
	return DerivationID(hexStr[:16])
}

// Derive appends a new step to the chain and returns the updated chain.
func (c DerivationChain) Derive(transform, sourceSystem string, lossMagnitude float64, detail string) DerivationChain {
	if lossMagnitude < 0 {
		lossMagnitude = 0
	}
	if lossMagnitude > 1 {
		lossMagnitude = 1
	}

	timestamp := time.Now().UTC()
	var parentID DerivationID
	if len(c.Steps) > 0 {
		parentID = c.Steps[len(c.Steps)-1].ID
	}

	stepID := generateDerivationID(parentID, transform, timestamp)

	newStep := DerivationStep{
		ID:            stepID,
		ParentID:      parentID,
		Transform:     transform,
		LossMagnitude: lossMagnitude,
		SourceSystem:  sourceSystem,
		Timestamp:     timestamp,
		Detail:        detail,
	}

	return DerivationChain{
		Steps: append(c.Steps, newStep),
	}
}

// Origin returns the first step in the chain, or nil if empty.
func (c DerivationChain) Origin() *DerivationStep {
	if len(c.Steps) == 0 {
		return nil
	}
	return &c.Steps[0]
}

// Depth returns the number of transformation hops from origin.
func (c DerivationChain) Depth() int {
	return len(c.Steps)
}

// TotalLoss returns the cumulative information loss (capped at 1.0).
// Computed as: 1 - product(1 - step.LossMagnitude) for all steps.
func (c DerivationChain) TotalLoss() float64 {
	if len(c.Steps) == 0 {
		return 0.0
	}

	product := 1.0
	for _, step := range c.Steps {
		product *= (1.0 - step.LossMagnitude)
	}

	loss := 1.0 - product
	if loss > 1.0 {
		return 1.0
	}
	if loss < 0.0 {
		return 0.0
	}
	return loss
}

// OriginDerivation creates a new chain with a single origin step.
func OriginDerivation(sourceSystem string) DerivationChain {
	timestamp := time.Now().UTC()
	stepID := generateDerivationID("", "origin", timestamp)

	return DerivationChain{
		Steps: []DerivationStep{
			{
				ID:            stepID,
				ParentID:      "",
				Transform:     "origin",
				LossMagnitude: 0.0,
				SourceSystem:  sourceSystem,
				Timestamp:     timestamp,
				Detail:        "",
			},
		},
	}
}

// Clone creates a deep copy of the derivation chain.
func (c DerivationChain) Clone() DerivationChain {
	if len(c.Steps) == 0 {
		return DerivationChain{Steps: []DerivationStep{}}
	}

	newSteps := make([]DerivationStep, len(c.Steps))
	copy(newSteps, c.Steps)
	return DerivationChain{Steps: newSteps}
}

// IsEmpty returns true if the chain has no steps.
func (c DerivationChain) IsEmpty() bool {
	return len(c.Steps) == 0
}

// LastTransform returns the name of the last transformation, or empty string if no steps.
func (c DerivationChain) LastTransform() string {
	if len(c.Steps) == 0 {
		return ""
	}
	return c.Steps[len(c.Steps)-1].Transform
}

// LastTimestamp returns the timestamp of the last transformation, or zero time if no steps.
func (c DerivationChain) LastTimestamp() time.Time {
	if len(c.Steps) == 0 {
		return time.Time{}
	}
	return c.Steps[len(c.Steps)-1].Timestamp
}

// DerivationSummary is a compact representation for serialization into context blocks.
type DerivationSummary struct {
	Depth        int     `json:"depth"`
	TotalLoss    float64 `json:"total_loss"`
	OriginSystem string  `json:"origin_system"`
	LastTransform string  `json:"last_transform"`
}

// Summary creates a lightweight summary of the derivation chain.
func (c DerivationChain) Summary() DerivationSummary {
	originSystem := ""
	if origin := c.Origin(); origin != nil {
		originSystem = origin.SourceSystem
	}

	return DerivationSummary{
		Depth:        c.Depth(),
		TotalLoss:    c.TotalLoss(),
		OriginSystem: originSystem,
		LastTransform: c.LastTransform(),
	}
}
