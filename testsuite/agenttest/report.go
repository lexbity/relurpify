package agenttest

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

func WriteCaseReport(path string, report CaseReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		return fmt.Errorf("mkdir report dir: %w", err)
	}
	if err := fs.WriteFileSecure(path, data); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}
