package plan

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFoldInManifestJSONRoundTripPreservesTimestamps(t *testing.T) {
	now := time.Now().UTC().Round(0)
	manifest := FoldInManifest{
		ManifestID:         "manifest-1",
		PlanID:             "plan-1",
		FeatureDescription: "fold in auth flow",
		StructuralReadiness: []ReadinessGap{{
			Description:  "extract shared middleware",
			Scope:        []string{"auth.middleware"},
			LinkedStepID: "step-2",
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	var decoded FoldInManifest
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, manifest.CreatedAt, decoded.CreatedAt)
	require.Equal(t, manifest.UpdatedAt, decoded.UpdatedAt)
	require.Equal(t, manifest.StructuralReadiness, decoded.StructuralReadiness)
}
