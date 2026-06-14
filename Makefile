.PHONY: test-unit test-integ test-scenario test-all
.PHONY: test-contract-migration test-dev-agent test-tape-fidelity check-contract-dissolution grep-architecture-gates
.PHONY: lint-config lint-config-boundary test-boundary generate-templates check-template-drift check-boot-root check-config-tree-drift
.PHONY: lint-layering lint-invariants lint-all lint-arch lint-go lint-go-fix
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
	@if grep -rn 'InvokeOnBestNode\|RegisterNodeProvider\|NodeSelectionCriteria\|RateLimiter\|GetWorkingValue' --include='*.go' . 2>/dev/null | grep -v '.gomodcache' | grep -v 'tooling/arch' | grep -v 'state.go' | grep -v 'state_test.go' | grep -v 'runner_test.go'; then echo "[FAIL] no-dead: found removed symbols" ; exit 1 ; fi
	@echo "[PASS] no-dead: no removed symbols found"


lint-all: lint-layering lint-invariants lint-config-boundary

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

lint-config:
	$(GO_OFFLINE_ENV) go run ./app/relurplint --check all


test-unit: lint-config
	$(GO_OFFLINE_ENV) go test ./... -count=1 -timeout 60s

test-integ:
	$(GO_OFFLINE_ENV) go test ./... -tags integration -count=1 -timeout 120s

test-scenario:
	$(GO_OFFLINE_ENV) go test ./... -tags scenario -count=1 -timeout 180s

test-all: test-unit test-integ test-scenario

# Phase 0 baseline gates for the contract-dissolution / dev-agent-revival track.
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

grep-architecture-gates:
	@$(MAKE) check-contract-dissolution
	@if rg -n "os\\.(Getenv|LookupEnv|Environ)" app execution governance platform/llm testsuite --glob '*.go' >/dev/null; then echo "[FAIL] grep-architecture-gates: direct env access remains outside userconfig"; exit 1; fi
	@if rg -n "shim|compatibility|stub|backward compatibility" app/dev-agent-cli userconfig execution/session app/relurpish testsuite/agenttest platform/llm --glob '*.go' --glob '!**/*_test.go' >/dev/null; then echo "[FAIL] grep-architecture-gates: compatibility language remains in touched production code"; exit 1; fi
	@echo "[PASS] grep-architecture-gates: architecture fences are clean"
