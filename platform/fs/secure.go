package fs

import "os"

const (
	SecureFileMode os.FileMode = 0o600
	SecureDirMode  os.FileMode = 0o700
	PublicFileMode os.FileMode = 0o644
	PublicDirMode  os.FileMode = 0o755
)

func WriteFileSecure(path string, data []byte) error {
	return os.WriteFile(path, data, SecureFileMode)
}

func MkdirAllSecure(path string) error {
	return os.MkdirAll(path, SecureDirMode)
}
