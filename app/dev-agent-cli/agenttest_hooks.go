package main

import (
	"context"
	"io"

	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

func init() {
	agenttest.PreparedRunExecutorFn = func(ctx context.Context, descriptorPath string, outputRoot string, serviceID string, out io.Writer) error {
		return executePreparedRunToWriter(ctx, descriptorPath, outputRoot, preparedRunOverrides{}, serviceID, out)
	}
}
