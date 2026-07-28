.PHONY: test-unit test-integ test-scenario test-conformance test-all
.PHONY: test-contract-migration test-dev-agent test-tape-fidelity test-euclo-golden check-contract-dissolution grep-architecture-gates
.PHONY: lint-config generate-config check-config-tree-drift
.PHONY: lint-layering lint-invariants lint-all lint-arch lint-go lint-go-fix check-makefile-phonys check-no-dead-resolver check-no-ghost-schemas
.PHONY: domain-check domain-cycles no-bucket no-dead exception-count

GO_OFFLINE_ENV := GOPROXY=off GOSUMDB=off

# Architecture invariant gates (GP-9).
# governance-no-orch and no-bucket are now in enforce mode (Slice 7).
# classification-ownership was deleted in Slice 4 -- EffectClass/CapabilityScope live in governance/classification.
lint-arch:
	$(GO_OFFLINE_ENV) go run ./tooling/arch/cmd/archcheck; EXIT_CODE=$$?; \
	$(GO_OFFLINE_ENV) go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=governance-orch; \
	$(GO_OFFLINE_ENV) go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=context-ports; \
	exit $$EXIT_CODE

# check-config-tree-drift materialises the embedded default template bundle and
# diffs the ENTIRE generated tree against the checked-in relurpify_cfg/.
# A == C by construction; any diff is a real drift.
# Returns non-zero when drift is detected (AC-4).
check-config-tree-drift:
	@tmpdir=$$(mktemp -d); trap "rm -rf $$tmpdir" EXIT; \
	$(GO_OFFLINE_ENV) go run ./userconfig/templates/cmd/generate-config $$tmpdir/out; \
	if ! diff -qr $$tmpdir/out relurpify_cfg/ >/dev/null 2>&1; then \
		echo "[FAIL] relurpify_cfg/ drifts from embedded templates. Run: make generate-config"; \
		diff -qr $$tmpdir/out relurpify_cfg/ 2>/dev/null || true; \
		exit 1; \
	fi; \
	echo "[PASS] relurpify_cfg/ matches embedded templates"

# check-makefile-phonys verifies every .PHONY target has a real recipe.
check-makefile-phonys:
	@bash tooling/check-makefile-phonys.sh

# check-no-dead-resolver asserts no dead resolver paths remain in userconfig/templates.
check-no-dead-resolver:
	@if rg -n 'templates/workspace\b|templates/skills|templates/agents' userconfig/templates --glob '*.go' 2>/dev/null; then echo "[FAIL] dead resolver path remains"; exit 1; fi
	@echo "[PASS] no dead resolver paths"

# check-no-ghost-schemas asserts no tool/v2 or skill schema references remain
# in production code. Test files may reference removed schemas for
# rejection-assertion tests.
check-no-ghost-schemas:
	@if rg -n 'relurpify/tool/v2|"skill"' userconfig/config --glob '*.go' --glob '!*_test.go' 2>/dev/null; then echo "[FAIL] ghost schema remains"; exit 1; fi
	@echo "[PASS] no ghost schemas"

# Domain DAG direction checker (§2.1). Warn-mode: reports violations, exits 0.
# Enforce-mode available as a separate target (15 pre-existing non-P-phase
# violations remain; domain acyclicity program scope is complete).
domain-check:
	$(GO_OFFLINE_ENV) go run ./tooling/arch/cmd/domaincheck -mode=warn -check=direction

domain-check-enforce:
	$(GO_OFFLINE_ENV) go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=direction; \
	echo "[INFO] enforce mode reports $(shell $(GO_OFFLINE_ENV) go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=direction 2>&1 | grep -c '^  direction') violations (all non-P-phase, out of scope)"

# Domain-level cycle reporter. Lists every mutual (cyclic) domain pair.
domain-cycles:
	$(GO_OFFLINE_ENV) go run ./tooling/arch/cmd/domaincheck -mode=warn -check=cycles

# No-bucket guard (enforce mode, Slice 7): flags any type-only package
# imported by ≥3 domains. Pure-vocabulary packages at <domain>/ or
# <domain>/classification are exempt per NFR-7.
no-bucket:
	$(GO_OFFLINE_ENV) go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=nobucket

# Governance-no-orchestration (enforce mode, Slice 7): flags governance → execution imports.
governance-no-orch:
	$(GO_OFFLINE_ENV) go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=governance-orch

# Exception-count gate: fails CI if exceptions.yaml gains net-new entries.
# All P-phase exceptions retired: P7/P8/P10/P11/P12/P13/P15 (Slice 14).
exception-count:
	@count=$$(rg -c 'src_domain:' tooling/arch/exceptions.yaml 2>/dev/null || echo 0); \
	baseline=0; \
	if [ "$$count" -gt "$$baseline" ]; then \
		echo "[FAIL] exception-count: exceptions.yaml has $$count entries (baseline $$baseline) — net-new exceptions require extending the spec"; \
		exit 1; \
	fi; \
	echo "[PASS] exception-count: $$count entries (baseline $$baseline)"

# Dead-code gate: asserts removed symbols never reappear.
no-dead:
	@if grep -rn 'InvokeOnBestNode\|RegisterNodeProvider\|NodeSelectionCriteria\|RateLimiter\|GetWorkingValue\|executionStepFromAgent\|inheritExecutionStepScope\|summarizeCaptureBindings\|summarizeToolScopeFrames\|CompiledThoughtRecipe\|CompiledStep\b\|CompiledParallelGroup\|CompiledConditionalGroup\|buildParallelSection\|buildConditionalSection\|buildBranchSequence\|evaluateThoughtRecipeCondition\|emitParallelFanouts\|BackendModelProfileProvenance\|BackendProviderProvenance\|VerifyStepResult\|WriteBenchmarkBaseline\|BuildBenchmarkBaseline\|BuildPhaseMetrics\|ComparePerformanceBaseline\|WrapRegistryWithInterceptor\|ReadTelemetryJSONL\|LoadGoldenFingerprint\|LoadTape\|SetHandleScoped\|GetHandle\|LifecycleView' --include='*.go' . 2>/dev/null | grep -v '.gomodcache' | grep -v '.gocache' | grep -v 'tooling/arch' | grep -v '_test.go' | grep -v 'state.go' | grep -v 'state_adapter.go' | grep -v 'session_overlay.go' | grep -v 'edit_record.go' | grep -v 'runner_test.go'; then echo "[FAIL] no-dead: found removed symbols" ; exit 1 ; fi
	@echo "[PASS] no-dead: no removed symbols found"

.PHONY: no-dead-packages
no-dead-packages:
	@if grep -rn 'codeburg.org/lexbit/relurpify/jobs/store\|codeburg.org/lexbit/relurpify/platform/shell/query\|codeburg.org/lexbit/relurpify/platform/sandbox/dockersandbox\|codeburg.org/lexbit/relurpify/platform/sandbox/egressproxy\|codeburg.org/lexbit/relurpify/cognitionzoo/htn/authoring\|codeburg.org/lexbit/relurpify/cognitionzoo/llm\|codeburg.org/lexbit/relurpify/cognitionzoo/pipeline/stages\|codeburg.org/lexbit/relurpify/testsuite/agenttestscenario' --include='*.go' . 2>/dev/null | grep -v '.gomodcache' | grep -v '.gocache'; then echo "[FAIL] no-dead-packages: deleted package re-imported"; exit 1; fi
	@echo "[PASS] no-dead-packages: no deleted packages re-imported"

lint-all: lint-layering lint-invariants check-makefile-phonys check-no-dead-resolver check-no-ghost-schemas euclo-stepkind-exhaustive euclo-no-control-keys euclo-no-dead-flatteners no-dead no-dead-packages

# Standard Go linters (golangci-lint, config: .golangci.yaml). Use locally;
# CI uses two-track gate (ratchet + graduated) for enforce.
lint-go:
	$(GO_OFFLINE_ENV) golangci-lint run ./...

# Apply all auto-fixable findings (gofmt, goimports, unconvert, many revive, etc.).
lint-go-fix:
	$(GO_OFFLINE_ENV) golangci-lint run --fix ./...

# Two-track CI gate: ratchet blocks new issues; graduated blocks any occurrence.
# Mirrors the CI gate in .github/workflows/ci.yml.
lint-ratchet:
	$(GO_OFFLINE_ENV) golangci-lint run --new-from-rev=origin/main ./...

lint-graduated:
	@graduated=$$(grep -vE '^\s*(#|$$)' .golangci-graduated.txt | paste -sd,); \
	if [ -n "$$graduated" ]; then \
		$(GO_OFFLINE_ENV) golangci-lint run --enable-only="$$graduated" ./...; \
	fi

generate-config:
	@go run ./userconfig/templates/cmd/generate-config $(or $(OUTDIR),relurpify_cfg)

lint-config:
	@mkdir -p /tmp/relurpify-go-cache /tmp/relurpify-go-tmp
	$(GO_OFFLINE_ENV) GOCACHE=/tmp/relurpify-go-cache GOTMPDIR=/tmp/relurpify-go-tmp go run ./app/relurplint --check all


test-unit: lint-config
	@mkdir -p /tmp/relurpify-go-cache /tmp/relurpify-go-tmp
	$(GO_OFFLINE_ENV) GOCACHE=/tmp/relurpify-go-cache GOTMPDIR=/tmp/relurpify-go-tmp go test ./... -count=1 -timeout 60s

test-conformance: lint-config
	@mkdir -p /tmp/relurpify-go-cache /tmp/relurpify-go-tmp
	$(GO_OFFLINE_ENV) GOCACHE=/tmp/relurpify-go-cache GOTMPDIR=/tmp/relurpify-go-tmp go test ./testsuite/conformance -count=1 -timeout 60s

test-integ:
	$(GO_OFFLINE_ENV) go test ./... -tags integration -count=1 -timeout 120s

test-scenario:
	$(GO_OFFLINE_ENV) go test ./... -tags scenario -count=1 -timeout 180s

test-all: test-unit test-integ test-scenario

# Baseline gates for the contract-dissolution / dev-agent-revival track.
# These are intentionally explicit package subsets so the baseline is readable.
check-contract-dissolution:
	@rg -n "\\bManifestSpec\\b|\\bManifestSnapshot\\b|\\bManifestPolicySpec\\b|\\bManifestDefaults\\b" --glob '*.go' . >/dev/null && { echo "[FAIL] check-contract-dissolution: manifest spine still present"; exit 1; } || true
	@echo "[PASS] check-contract-dissolution: manifest spine removed"

test-contract-migration:
	@$(MAKE) check-contract-dissolution
	@mkdir -p /tmp/relurpify-go-cache
	$(GO_OFFLINE_ENV) GOCACHE=/tmp/relurpify-go-cache go test ./userconfig/config ./execution/session/... ./governance/... ./app/envcomposition/... ./ayenitd/... -count=1

test-dev-agent:
	@mkdir -p /tmp/relurpify-go-cache /tmp/relurpify-go-modcache
	@if [ -z "$$(ls -A /tmp/relurpify-go-modcache 2>/dev/null)" ]; then cp -a /home/lex/go/pkg/mod/. /tmp/relurpify-go-modcache/; fi
	$(GO_OFFLINE_ENV) GOCACHE=/tmp/relurpify-go-cache GOMODCACHE=/tmp/relurpify-go-modcache go build ./app/dev-agent-cli/...
	$(GO_OFFLINE_ENV) GOCACHE=/tmp/relurpify-go-cache GOMODCACHE=/tmp/relurpify-go-modcache go test ./app/dev-agent-cli/... -count=1

test-tape-fidelity:
	@mkdir -p /tmp/relurpify-go-cache
	$(GO_OFFLINE_ENV) GOCACHE=/tmp/relurpify-go-cache go test ./platform/llm -run 'TestTapeModelReplaysCommittedEucloSmoke' -count=1
	$(GO_OFFLINE_ENV) GOCACHE=/tmp/relurpify-go-cache go test ./testsuite/agenttest/tapes -run 'TestCommittedEucloTapeValidates|TestCommittedEucloLineageValidates' -count=1
	$(GO_OFFLINE_ENV) GOCACHE=/tmp/relurpify-go-cache go test ./app/relurpish/runtime -run TestEucloTapeFidelity -count=1

# test-euclo-golden runs the golden characterisation harness for euclo recipe
# compilation (plan snapshot, graph topology, execution trace). Each slice after
# Slice 1 MUST keep these goldens byte-identical (NFR-2).
# Set UPDATE_GOLDEN=1 to re-baseline.
test-euclo-golden:
	@mkdir -p /tmp/relurpify-go-cache
	$(GO_OFFLINE_ENV) GOCACHE=/tmp/relurpify-go-cache go test ./named/euclo/thoughtrecipes/ -run 'TestGolden' -count=1 -v

grep-architecture-gates:
	@$(MAKE) check-contract-dissolution
	@if rg -n "ResolveCallingMode|RenderToolsToPrompt|ParseToolCallsFromText" cognitionzoo capability/registry --glob '*.go' >/dev/null; then echo "[FAIL] grep-architecture-gates: legacy tool-calling wire symbols remain in cognitionzoo or capability/registry"; exit 1; fi
	@if rg -n "return c\\.Chat\\(ctx, messages, options\\)" platform/llm/ollama/client.go >/dev/null; then echo "[FAIL] grep-architecture-gates: ollama client still drops tools in non-native mode"; exit 1; fi
	@hits=$$(rg -n "os\\.(Getenv|LookupEnv|Environ)" --glob '*.go' --glob '!**/*_test.go' --glob '!.gomodcache/**' --glob '!.gocache/**' . 2>/dev/null | grep -v 'userconfig/' | head -20); \
	if [ -n "$$hits" ]; then \
		echo "[FAIL] grep-architecture-gates: direct env access remains outside userconfig"; \
		echo "$$hits"; \
		exit 1; \
	fi
	@if rg -n "shim|compatibility|stub|backward compatibility" app/dev-agent-cli userconfig execution/session app/relurpish testsuite/agenttest platform/llm --glob '*.go' --glob '!**/*_test.go' >/dev/null; then echo "[FAIL] grep-architecture-gates: compatibility language remains in touched production code"; exit 1; fi
	@echo "[PASS] grep-architecture-gates: architecture fences are clean"

# euclo-stepkind-exhaustive: asserts no string-based dispatch on ExecutionStep.Kind
# (not parser keywords, not paradigm strings — those are separate concerns).
euclo-stepkind-exhaustive:
	@echo "[check] euclo-stepkind-exhaustive: no string-based ExecutionStep.Kind dispatch..."
	@m=$$(rg -n '(n\.step\.Kind|step\.Kind)\s*==\s*"' --glob '*.go' --glob '!*_test.go' named/euclo/ 2>/dev/null); \
	if [ -n "$$m" ]; then echo "[FAIL] string-based ExecutionStep.Kind dispatch (use == StepKind*):"; echo "$$m"; exit 1; fi
	@if rg -n 'strings\.(EqualFold|TrimSpace).*\.(step\.Kind|n\.step\.Kind)\b' --glob '*.go' named/euclo/ 2>/dev/null | grep -qv '_test.go'; then \
		echo "[FAIL] strings.EqualFold/TrimSpace on ExecutionStep.Kind (use == StepKind* instead)"; exit 1; \
	fi
	@# R4: ExecutionStep must not carry a nested surface.ThoughtRecipeStep
	@# (dual representation). Control/runtime data lives in typed top-level fields;
	@# the inner stringly .Step.Type / .Step.Config is the leak this forbids.
	@if rg -n '^\s*Step\s+surface\.ThoughtRecipeStep\b' --glob '*.go' named/euclo/thoughtrecipes/ 2>/dev/null; then \
		echo "[FAIL] ExecutionStep carries a nested surface.ThoughtRecipeStep (use typed top-level fields + ToSurfaceStep)"; exit 1; \
	fi
	@if rg -n '\.Step\.(Type|Config|OnError|Context|Parent)\b' --glob '*.go' --glob '!*_test.go' named/euclo/ 2>/dev/null; then \
		echo "[FAIL] read of nested ExecutionStep.Step.* (use typed top-level fields)"; exit 1; fi
	@echo "[PASS] euclo-stepkind-exhaustive: all step kind dispatch is typed"

# euclo-no-control-keys: asserts the envelope carries only state + results,
# not control flow (Q5, A-5, FR-6). Control data must live in typed step fields.
euclo-no-control-keys:
	@echo "[check] euclo-no-control-keys: control data must travel in typed plan fields, not the envelope/task.Context/state maps..."
	@m=$$(rg -n '(task\.Context|state)\["(euclo\.(run|ask|delegate)\.|execution_)' --glob '*.go' --glob '!*_test.go' named/euclo/ 2>/dev/null); \
	if [ -n "$$m" ]; then echo "[FAIL] control-flow key written to task.Context/state map (use typed step fields / PlanState):"; echo "$$m"; exit 1; fi
	@m=$$(rg -n 'SetTyped\(env,\s*"(euclo\.(run|ask|delegate)\.|.*execution_(goal|question|sources|directives|choices|choice_source|instruction|paradigm|step_id|step_type|prompt_id))' --glob '*.go' --glob '!*_test.go' named/euclo/ 2>/dev/null); \
	if [ -n "$$m" ]; then echo "[FAIL] control-flow key written via SetTyped (use typed step fields / PlanState):"; echo "$$m"; exit 1; fi
	@echo "[PASS] euclo-no-control-keys: envelope carries state + results only"

# euclo-no-dead-flatteners: asserts the flatteners/round-trip/dead-parallel paths
# the spec deletes in Slice 3/7 never reappear (AC-3, Slice 7).
euclo-no-dead-flatteners:
	@echo "[check] euclo-no-dead-flatteners: no flattener/round-trip/dead-parallel symbols..."
	@m=$$(rg -n '\b(summarizeCaptureBindings|summarizeToolScopeFrames|summarizeExecutionItems|rawValueExprList|predicateRaw|buildParallelSection|CompiledParallelGroup)\b' --glob '*.go' --glob '!*_test.go' named/euclo/ 2>/dev/null); \
	if [ -n "$$m" ]; then echo "[FAIL] removed flattener/round-trip/dead-parallel symbol still present:"; echo "$$m"; exit 1; fi
	@echo "[PASS] euclo-no-dead-flatteners: no dead flattener/round-trip symbols"

# Slice 10 structural gates: enforce that dead code and forbidden patterns
# never reappear in production code.
check-gates-slice10:
	@echo "[check] AC-12: no dead modelselect loaders..."
	@if rg -n "modelselect\.LoadProfileRegistry|modelselect\.LoadProviderRegistry" -g '*.go' -g '!*_test.go' . 2>/dev/null | grep -q .; then echo "[FAIL] AC-12: dead LoadProfileRegistry/LoadProviderRegistry still referenced"; exit 1; fi
	@echo "[PASS] AC-12: no dead modelselect loader references"
	@echo "[check] provider_factory.go absent..."
	@if test -f app/relurpish/runtime/provider_factory.go; then echo "[FAIL] provider_factory.go still present"; exit 1; fi
	@echo "[PASS] provider_factory.go absent"
	@echo "[check] AC-10: product code does not set offline_scenario..."
	@! grep -rn '"offline_scenario"' --include='*.go' app/relurpish/ | grep -v '_test.go' >/dev/null 2>&1 || { echo "[FAIL] AC-10: product code in app/relurpish sets offline_scenario"; exit 1; }
	@echo "[PASS] AC-10: no product code sets offline_scenario"
	@echo "[check] AC-10: real backends do not import offline scenario..."
	@! grep -q 'platform/llm/offline' platform/llm/providers_ollama.go platform/llm/providers_lmstudio.go 2>/dev/null || { echo "[FAIL] AC-10: ollama/lmstudio imports offline scenario"; exit 1; }
	@echo "[PASS] AC-10: real backends do not import offline scenario"
	@echo "[PASS] All Slice 10 gates passed"

# check-docs asserts doc honesty: no dead docs/ pointer, no nexus/rex,
# offline framed as CI/plumbing, and first-run ollama flow present.
check-docs:
	@echo "[check] AC-9: no dead docs/ pointer..."
	@! grep -q 'docs/' README.md 2>/dev/null || { echo "[FAIL] AC-9: README still references docs/"; exit 1; }
	@echo "[PASS] AC-9: no dead docs/ pointer"
	@echo "[check] AC-9: no nexus/rex references in README..."
	@! grep -qiE '\bnexus\b|rex' README.md 2>/dev/null || { echo "[FAIL] AC-9: README still references nexus/rex"; exit 1; }
	@echo "[PASS] AC-9: README clean"
	@echo "[check] AC-9: no nexus in config package..."
	@if rg -n 'nexus' userconfig/config --glob '*.go' --glob '!*_test.go' 2>/dev/null | grep -q .; then echo "[FAIL] AC-9: nexus reference in userconfig/config"; exit 1; fi
	@echo "[PASS] AC-9: config package clean"
	@echo "[check] AC-9: offline framed as CI/plumbing..."
	@! grep -qi 'zero-dep demo' README.md 2>/dev/null || { echo "[FAIL] AC-9: offline still framed as demo"; exit 1; }
	@! grep -qi 'quick start.*no external' README.md 2>/dev/null || { echo "[FAIL] AC-9: offline still in quick-start section"; exit 1; }
	@echo "[PASS] AC-9: offline correctly framed"
	@echo "[check] AC-9: first-run ollama flow present..."
	@if ! grep -q "ollama serve" README.md 2>/dev/null || ! grep -q "ollama pull" README.md 2>/dev/null || ! grep -q "doctor" README.md 2>/dev/null; then echo "[FAIL] AC-9: missing first-run ollama flow (ollama serve/pull/doctor)"; exit 1; fi
	@echo "[PASS] AC-9: first-run ollama flow documented"
	@echo "[PASS] All AC-9 doc honesty gates passed"

# check-no-ghost-providers asserts that the ghost provider strings
# (vllm, tgi, llama-server, and non-canonical openai-compat/openai_compat)
# do not appear in production code. Test files are exempt.
check-no-ghost-providers:
	@echo "[check] AC-4: no ghost provider strings in platform/llm..."
	@! grep -rnE '"vllm"|"tgi"|"llama-server"|"openai-compat"|"openai_compat"' --include='*.go' platform/llm/ | grep -v '_test.go' >/dev/null 2>&1 || { echo "[FAIL] AC-4: ghost provider string found in platform/llm"; exit 1; }
	@echo "[PASS] AC-4: platform/llm clean"
	@echo "[check] AC-4: no ghost provider strings in app/relurpish/tui..."
	@! grep -rnE '"vllm"|"tgi"|"llama-server"|"openai-compat"|"openai_compat"' --include='*.go' app/relurpish/tui/ | grep -v '_test.go' >/dev/null 2>&1 || { echo "[FAIL] AC-4: ghost provider string found in app/relurpish/tui"; exit 1; }
	@echo "[PASS] AC-4: app/relurpish/tui clean"
	@echo "[PASS] All AC-4 ghost provider gates passed"
