package browser

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
)

type browserPaths struct {
	serviceRoot   string
	launchRoot    string
	profilesRoot  string
	sessionsRoot  string
	downloadsRoot string
	cacheRoot     string
	crashRoot     string
	metadataRoot  string
	logsRoot      string
}

type browserSessionPaths struct {
	profileDir   string
	downloadDir  string
	cacheDir     string
	crashDir     string
	metadataFile string
	logFile      string
}

func newBrowserPaths(workspaceRoot string) browserPaths {
	base := filepath.Join(config.New(workspaceRoot).ConfigRoot(), "browser")
	return browserPaths{
		serviceRoot:   base,
		launchRoot:    filepath.Join(base, "launch"),
		profilesRoot:  filepath.Join(base, "profiles"),
		sessionsRoot:  filepath.Join(base, "sessions"),
		downloadsRoot: filepath.Join(base, "downloads"),
		cacheRoot:     filepath.Join(base, "cache"),
		crashRoot:     filepath.Join(base, "crash"),
		metadataRoot:  filepath.Join(base, "metadata"),
		logsRoot:      filepath.Join(base, "logs"),
	}
}

func (p browserPaths) roots() map[string]string {
	return map[string]string{
		"service_root":   p.serviceRoot,
		"launch_root":    p.launchRoot,
		"profiles_root":  p.profilesRoot,
		"sessions_root":  p.sessionsRoot,
		"downloads_root": p.downloadsRoot,
		"cache_root":     p.cacheRoot,
		"crash_root":     p.crashRoot,
		"metadata_root":  p.metadataRoot,
		"logs_root":      p.logsRoot,
	}
}

func (p browserPaths) session(sessionID string) browserSessionPaths {
	sessionID = sanitizeBrowserPathSegment(sessionID)
	return browserSessionPaths{
		profileDir:   filepath.Join(p.profilesRoot, sessionID),
		downloadDir:  filepath.Join(p.downloadsRoot, sessionID),
		cacheDir:     filepath.Join(p.cacheRoot, sessionID),
		crashDir:     filepath.Join(p.crashRoot, sessionID),
		metadataFile: filepath.Join(p.metadataRoot, sessionID+".json"),
		logFile:      filepath.Join(p.logsRoot, sessionID+".log"),
	}
}

func (p browserSessionPaths) roots() map[string]string {
	return map[string]string{
		"profile_dir":   p.profileDir,
		"download_dir":  p.downloadDir,
		"cache_dir":     p.cacheDir,
		"crash_dir":     p.crashDir,
		"metadata_file": p.metadataFile,
		"log_file":      p.logFile,
	}
}

func (s *BrowserService) ensureBrowserPathRoot(label, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("browser %s missing", label)
	}
	if err := s.checkFileScope(core.FileSystemWrite, path); err != nil {
		return fmt.Errorf("browser %s out of scope: %w", label, err)
	}
	return nil
}

func sanitizeBrowserPathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	value = strings.ReplaceAll(value, string(filepath.Separator), "_")
	value = strings.ReplaceAll(value, "..", "_")
	return value
}
