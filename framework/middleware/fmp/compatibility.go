package fmp

import (
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
