package agenttest

import (
	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
)

type testStoreBundle struct {
	WorkflowStore agentlifecycle.Repository
	cleanup       func()
}

func (b *testStoreBundle) Close() {
	if b.cleanup != nil {
		b.cleanup()
	}
}

func newTestStoreBundle() (*testStoreBundle, error) {
	return &testStoreBundle{cleanup: func() {}}, nil
}
