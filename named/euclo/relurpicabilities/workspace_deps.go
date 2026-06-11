package relurpicabilities

import (
	"context"
	"os"
)

// WorkspaceFiles provides scoped file read/write/resolve for workspace mutation handlers.
type WorkspaceFiles interface {
	Resolve(candidate string) (abs string, rel string, err error)
	Read(candidate string) ([]byte, string, error)
	Write(candidate string, content []byte, perm os.FileMode) (string, error)
}

// IndexRefresher refreshes the index for a set of file paths.
type IndexRefresher interface {
	RefreshFiles(ctx context.Context, paths []string) error
}
