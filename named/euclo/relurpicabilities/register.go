package relurpicabilities

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

// RegisterAll registers all Euclo relurpic capability handlers with the
// capability registry in the provided workspace environment.
//
// This function is called during agent initialization.
func RegisterAll(env agentenv.WorkspaceEnvironment) error {
	if env.Registry == nil {
		return fmt.Errorf("capability registry is nil")
	}

	specs := []relurpicCapabilitySpec{
		{Handler: NewTestRunHandler(env), RequiredTools: []string{"file_read"}},
		{Handler: NewASTQueryHandler(env), RequiredTools: []string{"file_read"}},
		{Handler: NewSymbolTraceHandler(env), RequiredTools: []string{"file_read"}},
		{Handler: NewCallGraphHandler(env), RequiredTools: []string{"file_read"}},
		{Handler: NewBlameTraceHandler(env), RequiredTools: []string{"file_read"}},
		{Handler: NewBisectHandler(env), RequiredTools: []string{"file_read"}},
		{Handler: NewCodeReviewHandler(env), RequiredTools: []string{"file_read"}},
		{Handler: NewDiffSummaryHandler(env), RequiredTools: []string{"file_read"}},
		{Handler: NewLayerCheckHandler(env), RequiredTools: []string{"file_read"}},
		{Handler: NewTargetedRefactorHandler(env), RequiredTools: []string{"file_read", "file_write"}},
		{Handler: NewRenameSymbolHandler(env), RequiredTools: []string{"file_read", "file_write"}},
		{Handler: NewAPICompatHandler(env), RequiredTools: []string{"file_read"}},
		{Handler: NewBoundaryReportHandler(env), RequiredTools: []string{"file_read"}},
		{Handler: NewCoverageCheckHandler(env), RequiredTools: []string{"file_read"}},
	}

	for _, spec := range specs {
		if err := registerRelurpicCapability(env.Registry, spec); err != nil {
			return fmt.Errorf("failed to register handler: %w", err)
		}
	}

	return nil
}
