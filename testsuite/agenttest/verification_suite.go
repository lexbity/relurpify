package agenttest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// VerificationSuite verifies a prepared run using only on-disk artifacts.
type VerificationSuite struct {
	Contract VerificationContract
}

func NewVerificationSuite(desc *PreparedRunDescriptor) (*VerificationSuite, error) {
	contract, err := BuildVerificationContract(desc)
	if err != nil {
		return nil, err
	}
	return &VerificationSuite{Contract: contract}, nil
}

func (v *VerificationSuite) Verify(ctx context.Context, prepared *PreparedRun, suite *Suite, c CaseSpec) (*PreparedRunVerificationReport, error) {
	if v == nil {
		return nil, fmt.Errorf("verification suite required")
	}
	if prepared == nil || prepared.Descriptor == nil {
		return nil, fmt.Errorf("prepared run required")
	}
	report, err := LoadCaseReport(v.Contract.ExecutionReportPath)
	if err != nil {
		return nil, err
	}
	verification, err := VerifyPreparedRun(ctx, prepared, *report, suite, c, nil)
	if err != nil {
		return nil, err
	}
	return verification, nil
}

func (v *VerificationSuite) ArtifactExists(path string) bool {
	if v == nil {
		return false
	}
	if filepath.Clean(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
