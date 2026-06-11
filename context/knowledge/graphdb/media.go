package graphdb

import (
	"context"
	"encoding/json"
	"fmt"
)

// MaxInlinePropsBytes is the maximum size of serialized Props that may
// be stored inline in a node record. Payloads exceeding this limit MUST
// be stored as external references (path, content hash, artifact ref).
const MaxInlinePropsBytes = 1 << 16 // 64 KB

// ErrPropsTooLarge is returned when Props JSON exceeds MaxInlinePropsBytes.
var ErrPropsTooLarge = fmt.Errorf("graphdb: props exceed %d byte limit", MaxInlinePropsBytes)

// FileMeta holds the indexed metadata for a file in a mixed-media
// workspace. Large payload bytes MUST NOT be stored in these records.
type FileMeta struct {
	Path        string `json:"path"`
	ContentHash string `json:"content_hash,omitempty"`
	MediaType   string `json:"media_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	MTimeUnix   int64  `json:"mtime_unix,omitempty"`
	ArtifactRef string `json:"artifact_ref,omitempty"` // external reference to payload
}

// IndexFileMeta creates or updates a node in the graph that represents
// a file or mixed-media resource. The Props field is built from the
// FileMeta and validated against MaxInlinePropsBytes.
func (e *Engine) IndexFileMeta(ctx context.Context, id string, sourceID string, labels []string, meta FileMeta) error {
	propsRaw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("graphdb: marshal file meta: %w", err)
	}
	if len(propsRaw) > MaxInlinePropsBytes {
		return ErrPropsTooLarge
	}
	stableID := "path:" + meta.Path
	kind := "file"
	if meta.MediaType != "" {
		kind = "file:" + meta.MediaType
	}
	allLabels := make([]string, 0, len(labels)+2)
	allLabels = append(allLabels, labels...)
	if meta.MediaType != "" {
		allLabels = append(allLabels, "media:"+meta.MediaType)
	}
	if meta.ContentHash != "" {
		allLabels = append(allLabels, "hash:"+meta.ContentHash)
	}

	return e.UpsertNode(ctx, NodeRecord{
		ID:       id,
		Kind:     NodeKind(kind),
		SourceID: sourceID,
		StableID: stableID,
		Labels:   allLabels,
		Props:    propsRaw,
	})
}

// QueryFileMetaByPath returns file metadata nodes whose path matches.
func (e *Engine) QueryFileMetaByPath(path string) []NodeRecord {
	return e.ListNodesByLabelPrefix("", "path:"+path)
}

// QueryFileMetaByMedia returns file metadata nodes with the given media type.
func (e *Engine) QueryFileMetaByMedia(mediaType string) []NodeRecord {
	return e.ListNodesByLabel("", "media:"+mediaType)
}

// QueryFileMetaByHash returns file metadata nodes with the given content hash.
func (e *Engine) QueryFileMetaByHash(hash string) []NodeRecord {
	return e.ListNodesByLabel("", "hash:"+hash)
}

// QueryFileMetaBySource returns file metadata nodes for the given source.
func (e *Engine) QueryFileMetaBySource(sourceID string) []NodeRecord {
	return e.NodesBySource(sourceID)
}

// FileMetaFromNode extracts a FileMeta from a node record's Props.
func FileMetaFromNode(n NodeRecord) (*FileMeta, error) {
	if len(n.Props) == 0 {
		return nil, fmt.Errorf("graphdb: node %s has no props", n.ID)
	}
	var meta FileMeta
	if err := json.Unmarshal(n.Props, &meta); err != nil {
		return nil, fmt.Errorf("graphdb: unmarshal file meta from node %s: %w", n.ID, err)
	}
	return &meta, nil
}

// GenerateSyntheticFileMeta creates a synthetic FileMeta from a file path,
// suitable for testing and benchmarking. It does not read the file content.
func GenerateSyntheticFileMeta(path string, mediaType string, size int64) FileMeta {
	return FileMeta{
		Path:        path,
		ContentHash: fmt.Sprintf("sha256:%x", []byte(path)),
		MediaType:   mediaType,
		SizeBytes:   size,
	}
}

// GenerateSyntheticRepo populates an engine with file metadata nodes
// representing a synthetic mixed-media repository. It returns the
// number of nodes created.
func GenerateSyntheticRepo(e *Engine, root string, fileCount int, filesPerType int) int {
	mediaTypes := []string{
		"text/x-go",
		"text/x-python",
		"text/markdown",
		"image/png",
		"image/jpeg",
		"application/json",
		"text/css",
		"text/html",
	}
	var created int
	for _, mt := range mediaTypes {
		for i := 0; i < filesPerType; i++ {
			suffix := extForMediaType(mt)
			filename := fmt.Sprintf("%s/file-%s-%d%s", root, mediaLabel(mt), i, suffix)
			id := fmt.Sprintf("file:sha256:%x", []byte(filename))
			meta := GenerateSyntheticFileMeta(filename, mt, int64(100+i*50))
			if err := e.IndexFileMeta(context.TODO(), id, root, nil, meta); err != nil {
				panic(err)
			}
			created++
		}
	}
	return created
}

func extForMediaType(mt string) string {
	switch mt {
	case "text/x-go":
		return ".go"
	case "text/x-python":
		return ".py"
	case "text/markdown":
		return ".md"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "application/json":
		return ".json"
	case "text/css":
		return ".css"
	case "text/html":
		return ".html"
	default:
		return ".bin"
	}
}

func mediaLabel(mt string) string {
	switch mt {
	case "text/x-go":
		return "src"
	case "text/x-python":
		return "src"
	case "text/markdown":
		return "doc"
	case "image/png":
		return "img"
	case "image/jpeg":
		return "img"
	case "application/json":
		return "cfg"
	case "text/css":
		return "style"
	case "text/html":
		return "page"
	default:
		return "asset"
	}
}
