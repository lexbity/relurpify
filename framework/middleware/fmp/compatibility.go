package fmp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func ValidateOfferCompatibility(runtime core.RuntimeDescriptor, offer core.HandoffOffer, destination core.ExportDescriptor, now time.Time) *core.TransferRefusal {
	if strings.TrimSpace(runtime.RuntimeID) == "" {
		return nil
	}
	if runtime.MaxContextSize > 0 && offer.ContextSizeBytes > runtime.MaxContextSize {
		return &core.TransferRefusal{Code: core.RefusalContextTooLarge, Message: "context exceeds runtime max size"}
	}
	if len(runtime.SupportedContextClasses) > 0 && !containsFoldString(runtime.SupportedContextClasses, offer.ContextClass) {
		return &core.TransferRefusal{Code: core.RefusalUnsupportedContext, Message: "runtime does not support context class"}
	}
	if len(destination.RequiredCompatibilityClasses) > 0 && !containsFoldString(destination.RequiredCompatibilityClasses, runtime.CompatibilityClass) {
		return &core.TransferRefusal{Code: core.RefusalIncompatibleRuntime, Message: "runtime compatibility class not accepted"}
	}
	if offer.SourceCompatibilityClass != "" && len(destination.RequiredCompatibilityClasses) > 0 && !containsFoldString(destination.RequiredCompatibilityClasses, offer.SourceCompatibilityClass) {
		return &core.TransferRefusal{Code: core.RefusalIncompatibleRuntime, Message: "source compatibility class not accepted by export"}
	}
	if !runtime.ExpiresAt.IsZero() && now.After(runtime.ExpiresAt) {
		return &core.TransferRefusal{Code: core.RefusalAdmissionClosed, Message: "runtime advertisement expired"}
	}
	return nil
}

func ValidateImportedContextCompatibility(runtime core.RuntimeDescriptor, manifest core.ContextManifest, sealed core.SealedContext) error {
	if runtime.MaxContextSize > 0 && manifest.SizeBytes > runtime.MaxContextSize {
		return fmt.Errorf("manifest size %d exceeds runtime max size %d", manifest.SizeBytes, runtime.MaxContextSize)
	}
	if len(runtime.SupportedContextClasses) > 0 && !containsFoldString(runtime.SupportedContextClasses, manifest.ContextClass) {
		return fmt.Errorf("runtime does not support manifest context class %s", manifest.ContextClass)
	}
	if len(runtime.SupportedEncryptionSuites) > 0 && !containsFoldString(runtime.SupportedEncryptionSuites, sealed.CipherSuite) {
		return fmt.Errorf("runtime does not support cipher suite %s", sealed.CipherSuite)
	}
	if strings.TrimSpace(manifest.SchemaVersion) != "" && !strings.EqualFold(strings.TrimSpace(manifest.SchemaVersion), "fmp.context.v1") {
		return fmt.Errorf("unsupported schema version %s", manifest.SchemaVersion)
	}
	return nil
}

// Phase 6.4: Version Skew Handling

// CompatibilityWindow defines acceptable version ranges per context class.
type CompatibilityWindow struct {
	ContextClass         string `json:"context_class" yaml:"context_class"`
	MinSchemaVersion     string `json:"min_schema_version,omitempty" yaml:"min_schema_version,omitempty"`
	MaxSchemaVersion     string `json:"max_schema_version,omitempty" yaml:"max_schema_version,omitempty"`
	MinRuntimeVersion    string `json:"min_runtime_version,omitempty" yaml:"min_runtime_version,omitempty"`
	MaxRuntimeVersion    string `json:"max_runtime_version,omitempty" yaml:"max_runtime_version,omitempty"`
}

// CompatibilityWindowStore manages version compatibility windows per context class.
type CompatibilityWindowStore interface {
	GetWindow(ctx context.Context, contextClass string) (*CompatibilityWindow, bool, error)
	UpsertWindow(ctx context.Context, window CompatibilityWindow) error
	ListWindows(ctx context.Context) ([]CompatibilityWindow, error)
	DeleteWindow(ctx context.Context, contextClass string) error
}

// ValidateVersionSkew checks if schemaVersion and runtimeVersion fall within the configured window.
// Uses lexicographic string comparison (safe for semver vX.Y.Z format).
func ValidateVersionSkew(window CompatibilityWindow, schemaVersion, runtimeVersion string) *core.TransferRefusal {
	if schemaVersion != "" && window.MinSchemaVersion != "" {
		if schemaVersion < window.MinSchemaVersion {
			return &core.TransferRefusal{
				Code:    core.RefusalIncompatibleRuntime,
				Message: fmt.Sprintf("schema version %s below minimum %s", schemaVersion, window.MinSchemaVersion),
			}
		}
	}
	if schemaVersion != "" && window.MaxSchemaVersion != "" {
		if schemaVersion > window.MaxSchemaVersion {
			return &core.TransferRefusal{
				Code:    core.RefusalIncompatibleRuntime,
				Message: fmt.Sprintf("schema version %s above maximum %s", schemaVersion, window.MaxSchemaVersion),
			}
		}
	}
	if runtimeVersion != "" && window.MinRuntimeVersion != "" {
		if runtimeVersion < window.MinRuntimeVersion {
			return &core.TransferRefusal{
				Code:    core.RefusalIncompatibleRuntime,
				Message: fmt.Sprintf("runtime version %s below minimum %s", runtimeVersion, window.MinRuntimeVersion),
			}
		}
	}
	if runtimeVersion != "" && window.MaxRuntimeVersion != "" {
		if runtimeVersion > window.MaxRuntimeVersion {
			return &core.TransferRefusal{
				Code:    core.RefusalIncompatibleRuntime,
				Message: fmt.Sprintf("runtime version %s above maximum %s", runtimeVersion, window.MaxRuntimeVersion),
			}
		}
	}
	return nil
}
