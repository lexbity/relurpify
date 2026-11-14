package securefile

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

// AbsentRevision distinguishes a missing file from a present empty source.
const AbsentRevision = "absent"

// ErrRevisionConflict means the file identity changed while opening it.
var ErrRevisionConflict = errors.New("policy file identity changed during read")

// ReadRegularWithin reads a bounded, single-link regular file under an already
// opened root. Missing files have an explicit revision; other errors never look
// like empty source. The handle pins the root across path renames.
func ReadRegularWithin(root *os.Root, name string, limit int64) (data []byte, revision string, err error) {
	if root == nil || !filepath.IsLocal(name) || filepath.Clean(name) != name || limit <= 0 || limit == math.MaxInt64 {
		return nil, "", fmt.Errorf("local file path and positive limit required")
	}
	before, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, AbsentRevision, nil
	}
	if err != nil {
		return nil, "", err
	}
	if !before.Mode().IsRegular() {
		return nil, "", fmt.Errorf("policy source must be a regular file")
	}
	file, err := openRegularWithin(root, name)
	if err != nil {
		return nil, "", err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	after, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if !os.SameFile(before, after) {
		return nil, "", ErrRevisionConflict
	}
	data, err = io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("policy source exceeds %d bytes", limit)
	}
	return data, ComputeDigest(data), nil
}
