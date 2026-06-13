package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/execution/session"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type contractCorpusCase struct {
	name       string
	fixture    string
	golden     string
	load       func(t *testing.T, path string) (*config.EffectiveAgentContract, error)
	specAssert func(t *testing.T, contract *config.EffectiveAgentContract)
}

type contractGoldenView struct {
	AgentID     string               `json:"agent_id"`
	SpecPresent bool                 `json:"spec_present"`
	AgentSpec   any                  `json:"agent_spec,omitempty"`
	Permissions any                  `json:"permissions"`
	Resources   any                  `json:"resources"`
	Security    any                  `json:"security"`
	Sources     config.SourceSummary `json:"sources"`
}

func TestContractGoldenCorpus(t *testing.T) {
	cases := []contractCorpusCase{
		{
			name:    "document-resolver",
			fixture: filepath.Join("testdata", "contracts", "manifest_current.yaml"),
			golden:  filepath.Join("testdata", "contracts", "manifest_current.golden.yaml"),
			load: func(t *testing.T, path string) (*config.EffectiveAgentContract, error) {
				t.Helper()
				snapshot, err := config.LoadDocument(path)
				if err != nil {
					return nil, err
				}
				return config.ResolveEffectiveAgentContract("/workspace/contracts/manifest", snapshot.Document, config.ResolveOptions{})
			},
			specAssert: func(t *testing.T, contract *config.EffectiveAgentContract) {
				t.Helper()
				require.NotNil(t, contract)
				require.NotNil(t, contract.AgentSpec, "document resolver should produce an agent spec")
			},
		},
		{
			name:    "document-assembler",
			fixture: filepath.Join("testdata", "contracts", "document_current.yaml"),
			golden:  filepath.Join("testdata", "contracts", "document_current.golden.yaml"),
			load: func(t *testing.T, path string) (*config.EffectiveAgentContract, error) {
				t.Helper()
				snapshot, err := config.LoadDocument(path)
				if err != nil {
					return nil, err
				}
				contract, err := session.AssembleContract(snapshot.Document)
				if err != nil {
					return nil, err
				}
				return contract, nil
			},
			specAssert: func(t *testing.T, contract *config.EffectiveAgentContract) {
				t.Helper()
				require.NotNil(t, contract)
				require.NotNil(t, contract.AgentSpec, "document assembly should produce an agent spec")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			contract, err := tc.load(t, tc.fixture)
			require.NoError(t, err)
			tc.specAssert(t, contract)

			actual, err := json.MarshalIndent(contractGoldenView{
				AgentID:     contract.AgentID,
				SpecPresent: false,
				AgentSpec:   contract.AgentSpec,
				Permissions: contract.Permissions,
				Resources:   contract.Resources,
				Security:    contract.Security,
				Sources:     contract.Sources,
			}, "", "  ")
			require.NoError(t, err)

			expected, err := os.ReadFile(filepath.Clean(tc.golden))
			require.NoError(t, err)

			var expectedValue any
			require.NoError(t, yaml.Unmarshal(expected, &expectedValue))

			var actualValue any
			require.NoError(t, yaml.Unmarshal(actual, &actualValue))

			require.Equal(t, expectedValue, actualValue)
		})
	}
}
