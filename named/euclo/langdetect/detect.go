package langdetect

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultMaxDepth   = 2
	defaultMaxEntries = 200
	readdirChunkSize  = 64
)

// WorkspaceLanguages describes which supported languages are present in a workspace.
type WorkspaceLanguages struct {
	Go     bool
	Python bool
	Rust   bool
	JS     bool // JavaScript or TypeScript
}

// IsEmpty reports whether no supported language was detected.
func (w WorkspaceLanguages) IsEmpty() bool {
	return !w.Go && !w.Python && !w.Rust && !w.JS
}

// Detected returns the detected language IDs in stable order.
func (w WorkspaceLanguages) Detected() []string {
	langs := make([]string, 0, 4)
	if w.Go {
		langs = append(langs, "go")
	}
	if w.Python {
		langs = append(langs, "python")
	}
	if w.Rust {
		langs = append(langs, "rust")
	}
	if w.JS {
		langs = append(langs, "js")
	}
	return langs
}

// Detect probes workspaceDir for language indicator files and returns the
// detected workspace languages. It never returns an error.
func Detect(workspaceDir string) WorkspaceLanguages {
	return detect(workspaceDir, defaultMaxDepth, defaultMaxEntries)
}

func detect(workspaceDir string, maxDepth, maxEntries int) WorkspaceLanguages {
	workspaceDir = strings.TrimSpace(workspaceDir)
	if workspaceDir == "" {
		return WorkspaceLanguages{}
	}

	info, err := os.Stat(workspaceDir)
	if err != nil || !info.IsDir() {
		return WorkspaceLanguages{}
	}

	remaining := maxEntries
	langs, matched, remaining := scanDir(workspaceDir, 0, maxDepth, remaining)
	if matched || !langs.IsEmpty() || remaining <= 0 {
		return langs
	}
	return langs
}

func scanDir(dir string, depth, maxDepth, remaining int) (WorkspaceLanguages, bool, int) {
	if remaining <= 0 || depth > maxDepth {
		return WorkspaceLanguages{}, false, remaining
	}

	f, err := os.Open(dir)
	if err != nil {
		return WorkspaceLanguages{}, false, remaining
	}
	defer f.Close()

	var langs WorkspaceLanguages
	matchedHere := false
	subdirs := make([]string, 0, 8)

	for remaining > 0 {
		chunkSize := readdirChunkSize
		if remaining < chunkSize {
			chunkSize = remaining
		}

		infos, err := f.Readdir(chunkSize)
		if len(infos) == 0 {
			if err != nil {
				break
			}
			break
		}

		for _, info := range infos {
			remaining--
			if info.IsDir() {
				if depth < maxDepth {
					subdirs = append(subdirs, filepath.Join(dir, info.Name()))
				}
				continue
			}
			if detectFile(info.Name(), &langs) {
				matchedHere = true
			}
			if remaining <= 0 {
				break
			}
		}

		if err != nil {
			break
		}
		if remaining <= 0 {
			break
		}
	}

	if matchedHere {
		return langs, true, remaining
	}

	if depth >= maxDepth {
		return langs, false, remaining
	}

	for _, subdir := range subdirs {
		if remaining <= 0 {
			break
		}
		subLangs, subMatched, nextRemaining := scanDir(subdir, depth+1, maxDepth, remaining)
		remaining = nextRemaining
		langs = mergeLanguages(langs, subLangs)
		if subMatched {
			// Continue scanning siblings only until the entry cap is reached so
			// multi-language workspaces can still be identified.
			continue
		}
	}

	return langs, !langs.IsEmpty(), remaining
}

func mergeLanguages(a, b WorkspaceLanguages) WorkspaceLanguages {
	return WorkspaceLanguages{
		Go:     a.Go || b.Go,
		Python: a.Python || b.Python,
		Rust:   a.Rust || b.Rust,
		JS:     a.JS || b.JS,
	}
}

