package toolcapabilities

import (
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

type toolWithKeys struct {
	contracts.Tool
	keys []string
}

func (t *toolWithKeys) ParamKeys() []string { return t.keys }

type toolWithoutKeys struct {
	contracts.Tool
}

func TestAssertParamKeysPassesWhenConsumedKeysExist(t *testing.T) {
	impl := &toolWithKeys{keys: []string{"pattern", "directory"}}
	params := []contracts.ToolParameter{
		{Name: "pattern", Type: "string"},
		{Name: "directory", Type: "string"},
	}
	err := AssertParamKeys(impl, "search_grep", params)
	require.NoError(t, err)
}

func TestAssertParamKeysFailsOnMissingKey(t *testing.T) {
	impl := &toolWithKeys{keys: []string{"pattern", "missing_key"}}
	params := []contracts.ToolParameter{
		{Name: "pattern", Type: "string"},
	}
	err := AssertParamKeys(impl, "bad_tool", params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad_tool")
	require.Contains(t, err.Error(), "missing_key")
}

func TestAssertParamKeysSkipsWhenNoParamKeysProvider(t *testing.T) {
	impl := &toolWithoutKeys{}
	err := AssertParamKeys(impl, "simple_tool", nil)
	require.NoError(t, err)
}

func TestAssertParamKeysSkipsWhenNoConsumedKeys(t *testing.T) {
	impl := &toolWithKeys{keys: nil}
	err := AssertParamKeys(impl, "no_params", nil)
	require.NoError(t, err)
}

func TestAssertParamKeysNormalizesNames(t *testing.T) {
	impl := &toolWithKeys{keys: []string{"working_directory"}}
	params := []contracts.ToolParameter{
		{Name: "working_directory", Type: "string"},
	}
	err := AssertParamKeys(impl, "go_test", params)
	require.NoError(t, err)
}

func TestAssertParamKeysOnConstructorPasses(t *testing.T) {
	manifest := contracts.ToolManifest{
		Name: "search_grep",
		Parameters: []contracts.ToolParameter{
			{Name: "pattern", Type: "string", Required: true},
		},
	}
	ctor := func(basePath string) contracts.Tool {
		return &toolWithKeys{keys: []string{"pattern"}}
	}
	err := AssertParamKeysOnConstructor("search_grep", ctor, manifest)
	require.NoError(t, err)
}

func TestAssertParamKeysOnConstructorFails(t *testing.T) {
	manifest := contracts.ToolManifest{
		Name: "bad_tool",
		Parameters: []contracts.ToolParameter{
			{Name: "declared", Type: "string"},
		},
	}
	ctor := func(basePath string) contracts.Tool {
		return &toolWithKeys{keys: []string{"undeclared"}}
	}
	err := AssertParamKeysOnConstructor("bad_tool", ctor, manifest)
	require.Error(t, err)
	require.Contains(t, err.Error(), "undeclared")
}

func TestAssertParamKeysOnConstructorNilCtor(t *testing.T) {
	err := AssertParamKeysOnConstructor("nil", nil, contracts.ToolManifest{Name: "nil"})
	require.NoError(t, err)
}
