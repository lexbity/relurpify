# End to End testsuite


# Running against Ollama 

(no AST)

go test ./app/relurpish/runtime -run 'TestBuildCapabilityRegistrySkipASTIndexSkipsSemanticBootstrap' -count=1
go test ./testsuite/agenttest -run TestBuildAgentPropagatesSkipASTIndexToBootstrap -count=1
go run ./app/dev-agent-cli agenttest run --help | rg 'skip-ast-index|bootstrap-timeout'
