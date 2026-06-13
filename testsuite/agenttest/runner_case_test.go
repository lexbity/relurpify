//go:build live
// +build live

package agenttest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	loaded_model = "loaded-model"
)


func TestModelProvenanceDigest(t *testing.T) {
	t.Run("nil provenance returns empty string", func(t *testing.T) {
		digest := modelProvenanceDigest(nil)
		require.Equal(t, "", digest)
	})

	t.Run("digest from top-level field", func(t *testing.T) {
		provenance := &BackendModelProvenance{
			Digest: "sha256:abc123",
		}
		digest := modelProvenanceDigest(provenance)
		require.Equal(t, "sha256:abc123", digest)
	})

	t.Run("digest from details map", func(t *testing.T) {
		provenance := &BackendModelProvenance{
			Details: map[string]any{
				"digest": "sha256:def456",
			},
		}
		digest := modelProvenanceDigest(provenance)
		require.Equal(t, "sha256:def456", digest)
	})

	t.Run("empty digest returns empty string", func(t *testing.T) {
		provenance := &BackendModelProvenance{
			Digest: "",
		}
		digest := modelProvenanceDigest(provenance)
		require.Equal(t, "", digest)
	})

	t.Run("missing digest returns empty string", func(t *testing.T) {
		provenance := &BackendModelProvenance{
			Details: map[string]any{
				"other": "value",
			},
		}
		digest := modelProvenanceDigest(provenance)
		require.Equal(t, "", digest)
	})
}

func TestModelProvenanceName(t *testing.T) {
	t.Run("nil provenance returns empty string", func(t *testing.T) {
		name := modelProvenanceName(nil)
		require.Equal(t, "", name)
	})

	t.Run("loaded name takes precedence", func(t *testing.T) {
		provenance := &BackendModelProvenance{
			LoadedName:  "loaded-name",
			LoadedModel: loaded_model,
		}
		name := modelProvenanceName(provenance)
		require.Equal(t, "loaded-name", name)
	})

	t.Run("falls back to loaded model", func(t *testing.T) {
		provenance := &BackendModelProvenance{
			LoadedModel: loaded_model,
		}
		name := modelProvenanceName(provenance)
		require.Equal(t, loaded_model, name)
	})

	t.Run("both empty returns empty string", func(t *testing.T) {
		provenance := &BackendModelProvenance{
			LoadedName:  "",
			LoadedModel: "",
		}
		name := modelProvenanceName(provenance)
		require.Equal(t, "", name)
	})
}
