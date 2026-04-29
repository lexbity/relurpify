package fmp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

)

func ValidateOfferCompatibility(runtime RuntimeDescriptor, offer HandoffOffer, destination ExportDescriptor, now time.Time) *TransferRefusal {
	if strings.TrimSpace(runtime.RuntimeID) == "" {
		return nil
	}
	if runtime.MaxContextSize > 0 && offer.ContextSizeBytes > runtime.MaxContextSize {
		return &TransferRefusal{Code: RefusalContextTooLarge, Message: "context exceeds runtime max size"}
	}
	if len(runtime.SupportedContextClasses) > 0 && !containsFoldString(runtime.SupportedContextClasses, offer.ContextClass) {
		return &TransferRefusal{Code: RefusalUnsupportedContext, Message: "runtime does not support context class"}
	}
	if len(destination.RequiredCompatibilityClasses) > 0 && !containsFoldString(destination.RequiredCompatibilityClasses, runtime.CompatibilityClass) {
		return &TransferRefusal{Code: RefusalIncompatibleRuntime, Message: "runtime compatibility class not accepted"}
	}
	if offer.SourceCompatibilityClass != "" && len(destination.RequiredCompatibilityClasses) > 0 && !containsFoldString(destination.RequiredCompatibilityClasses, offer.SourceCompatibilityClass) {
		return &TransferRefusal{Code: RefusalIncompatibleRuntime, Message: "source compatibility class not accepted by export"}
	}
	if !runtime.ExpiresAt.IsZero() && now.After(runtime.ExpiresAt) {
		return &TransferRefusal{Code: RefusalAdmissionClosed, Message: "runtime advertisement expired"}
	}
	return nil
}

func ValidateImportedContextCompatibility(runtime RuntimeDescriptor, manifest ContextManifest, sealed SealedContext) error {
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
	ContextClass      string `json:"context_class" yaml:"context_class"`
	MinSchemaVersion  string `json:"min_schema_version,omitempty" yaml:"min_schema_version,omitempty"`
	MaxSchemaVersion  string `json:"max_schema_version,omitempty" yaml:"max_schema_version,omitempty"`
	MinRuntimeVersion string `json:"min_runtime_version,omitempty" yaml:"min_runtime_version,omitempty"`
	MaxRuntimeVersion string `json:"max_runtime_version,omitempty" yaml:"max_runtime_version,omitempty"`
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
func ValidateVersionSkew(window CompatibilityWindow, schemaVersion, runtimeVersion string) *TransferRefusal {
	if schemaVersion != "" && window.MinSchemaVersion != "" {
		if compareNaturalVersion(schemaVersion, window.MinSchemaVersion) < 0 {
			return &TransferRefusal{
				Code:    RefusalIncompatibleRuntime,
				Message: fmt.Sprintf("schema version %s below minimum %s", schemaVersion, window.MinSchemaVersion),
			}
		}
	}
	if schemaVersion != "" && window.MaxSchemaVersion != "" {
		if compareNaturalVersion(schemaVersion, window.MaxSchemaVersion) > 0 {
			return &TransferRefusal{
				Code:    RefusalIncompatibleRuntime,
				Message: fmt.Sprintf("schema version %s above maximum %s", schemaVersion, window.MaxSchemaVersion),
			}
		}
	}
	if runtimeVersion != "" && window.MinRuntimeVersion != "" {
		if compareNaturalVersion(runtimeVersion, window.MinRuntimeVersion) < 0 {
			return &TransferRefusal{
				Code:    RefusalIncompatibleRuntime,
				Message: fmt.Sprintf("runtime version %s below minimum %s", runtimeVersion, window.MinRuntimeVersion),
			}
		}
	}
	if runtimeVersion != "" && window.MaxRuntimeVersion != "" {
		if compareNaturalVersion(runtimeVersion, window.MaxRuntimeVersion) > 0 {
			return &TransferRefusal{
				Code:    RefusalIncompatibleRuntime,
				Message: fmt.Sprintf("runtime version %s above maximum %s", runtimeVersion, window.MaxRuntimeVersion),
			}
		}
	}
	return nil
}

func compareNaturalVersion(left, right string) int {
	leftParts := naturalVersionParts(left)
	rightParts := naturalVersionParts(right)
	maxParts := len(leftParts)
	if len(rightParts) > maxParts {
		maxParts = len(rightParts)
	}
	for i := 0; i < maxParts; i++ {
		lp, rp := partAt(leftParts, i), partAt(rightParts, i)
		if lp.numeric && rp.numeric {
			if lp.number < rp.number {
				return -1
			}
			if lp.number > rp.number {
				return 1
			}
			continue
		}
		if lp.numeric != rp.numeric {
			if lp.numeric {
				return 1
			}
			return -1
		}
		if lp.text < rp.text {
			return -1
		}
		if lp.text > rp.text {
			return 1
		}
	}
	return 0
}

type naturalVersionPart struct {
	numeric bool
	number  int
	text    string
}

func naturalVersionParts(value string) []naturalVersionPart {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return nil
	}
	parts := make([]naturalVersionPart, 0, 8)
	var current strings.Builder
	modeNumeric := false
	hasMode := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		text := current.String()
		part := naturalVersionPart{text: text}
		if modeNumeric {
			part.numeric = true
			part.number, _ = strconv.Atoi(text)
		}
		parts = append(parts, part)
		current.Reset()
	}
	for _, r := range value {
		if unicode.IsDigit(r) {
			if hasMode && !modeNumeric {
				flush()
			}
			modeNumeric = true
			hasMode = true
			current.WriteRune(r)
			continue
		}
		if unicode.IsLetter(r) {
			if hasMode && modeNumeric {
				flush()
			}
			modeNumeric = false
			hasMode = true
			current.WriteRune(r)
			continue
		}
		flush()
		hasMode = false
	}
	flush()
	return parts
}

func partAt(parts []naturalVersionPart, index int) naturalVersionPart {
	if index >= len(parts) {
		return naturalVersionPart{}
	}
	return parts[index]
}
