// Package securefile provides the secure file primitives the config domain
// needs to persist configuration and runtime state: directories private to
// the user (0700), files readable only by the owner (0600).
//
// The config domain owns these directly instead of importing the platform
// file capability: platform adapters consume this domain's declarative
// permission vocabulary, so a userconfig → platform edge would close a
// domain cycle. The two implementations are independent by design; the mode
// contract (0600 files, 0700 dirs) is the shared commitment.
package securefile

import "os"

// PublicDirMode is the mode for directories whose existence is not secret
// (e.g. the parent directories of shared documents).
const PublicDirMode os.FileMode = 0o755

// WriteFileSecure writes data to path, creating it with owner-only read/write.
func WriteFileSecure(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

// MkdirAllSecure creates path and any missing parents, owner-only access.
func MkdirAllSecure(path string) error {
	return os.MkdirAll(path, 0o700)
}
