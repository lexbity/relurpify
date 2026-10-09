package sandbox

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// PinSource describes how an image reference was pinned (SBH-1 D-13).
type PinSource string

const (
	// PinExplicit: the configured ref already carried @sha256:.
	PinExplicit PinSource = "explicit"
	// PinConfigured: security.sandbox.image_digest was stated by the operator.
	PinConfigured PinSource = "configured"
	// PinInspected: the local daemon resolved the ref to a repo digest.
	PinInspected PinSource = "inspected"
	// PinUnpinned: no digest could be resolved; the tag is used with a loud
	// event (availability over purity, made visible).
	PinUnpinned PinSource = "unpinned"
)

// ImagePin reports the resolved image reference and how it was pinned.
type ImagePin struct {
	Ref    string
	Source PinSource
}

// ResolveImageRef pins a container image by digest with the precedence
// (SBH-1 D-13): an explicit @sha256: ref > the configured image_digest >
// a local `docker image inspect` resolution > the plain tag with a loud
// unpinned signal. Never pulls; an inspect failure degrades to unpinned.
func ResolveImageRef(ctx context.Context, binary, ref, configuredDigest string) ImagePin {
	if strings.Contains(ref, "@sha256:") {
		return ImagePin{Ref: ref, Source: PinExplicit}
	}
	if digest := strings.TrimSpace(configuredDigest); digest != "" {
		return ImagePin{Ref: stripTagSuffix(ref) + "@" + digest, Source: PinConfigured}
	}
	inspectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resolved, err := resolveRepoDigest(inspectCtx, binary, ref)
	if err == nil && strings.HasPrefix(resolved, "sha256:") {
		return ImagePin{Ref: stripTagSuffix(ref) + "@" + resolved, Source: PinInspected}
	}
	return ImagePin{Ref: ref, Source: PinUnpinned}
}

// resolveRepoDigest asks the local daemon for the first RepoDigest of a ref.
func resolveRepoDigest(ctx context.Context, binary, ref string) (string, error) {
	if strings.TrimSpace(binary) == "" {
		binary = "docker"
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, path,
		"image", "inspect", "--format", "{{index .RepoDigests 0}}", ref,
	).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// stripTagSuffix removes a trailing :tag / @digest component so the digest
// can be appended cleanly.
func stripTagSuffix(ref string) string {
	if idx := strings.LastIndex(ref, "@"); idx > 0 {
		return ref[:idx]
	}
	if idx := strings.LastIndex(ref, ":"); idx > strings.LastIndexByte(ref, '/') && idx > 0 {
		return ref[:idx]
	}
	return ref
}

// EmitImagePinEvent records sandbox.image_pinned / sandbox.image_unpinned.
// Exported so the composition root surfaces pin status without re-deriving it.
func EmitImagePinEvent(ctx context.Context, sink telemetry.Telemetry, pin ImagePin) {
	eventType := telemetry.EventSandboxImagePinned
	message := "sandbox image pinned by digest"
	if pin.Source == PinUnpinned {
		eventType = telemetry.EventSandboxImageUnpinned
		message = "sandbox image unpinned — tag-based (set security.sandbox.image_digest)"
	}
	emitCommandEvent(ctx, sink, eventType, message, map[string]any{
		"source": string(pin.Source),
		"ref":    pin.Ref,
	})
}
