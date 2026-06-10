.PHONY: test-unit test-integ test-scenario test-all
.PHONY: lint-config lint-config-boundary test-boundary generate-templates check-template-drift check-boot-root check-config-tree-drift
.PHONY: lint-layering lint-invariants lint-all lint-arch lint-go lint-go-fix
.PHONY: domain-check domain-cycles no-bucket no-dead exception-count

# Architecture invariant gates (GP-9). Replaces the shelved scripts/boundaryaudit.
# governance-no-orch and no-bucket are now in enforce mode (Slice 7).
# classification-ownership was deleted in Slice 4 -- EffectClass/CapabilityScope live in governance/classification.
lint-arch:
	go run ./tooling/arch/cmd/archcheck; EXIT_CODE=$$?; \
	go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=governance-orch; \
	go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=context-ports; \
	exit $$EXIT_CODE

# Domain DAG direction checker (§2.1). Warn-mode: reports violations, exits 0.
domain-check:
	go run ./tooling/arch/cmd/domaincheck -mode=warn -check=direction

# Domain-level cycle reporter. Lists every mutual (cyclic) domain pair.
domain-cycles:
	go run ./tooling/arch/cmd/domaincheck -mode=warn -check=cycles

# No-bucket guard (enforce mode, Slice 7): flags any type-only package
# imported by ≥3 domains. Pure-vocabulary packages at <domain>/ or
# <domain>/classification are exempt per NFR-7.
no-bucket:
	go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=nobucket

# Governance-no-orchestration (enforce mode, Slice 7): flags governance → execution imports.
governance-no-orch:
	go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=governance-orch

# Exception-count gate: fails CI if exceptions.yaml gains net-new entries.
# Current baseline: 1 direction violation (P11).
# P6 and P14 were retired by Slices 1 and 3 respectively.
# P7/P8/P10/P12 were retired by Slice 1; P13 by Slice 3; P15 by Slice 8.
exception-count:
	@count=$$(rg -c 'src_domain:' tooling/arch/exceptions.yaml 2>/dev/null || echo 0); \
	baseline=1; \
	if [ "$$count" -gt "$$baseline" ]; then \
		echo "[FAIL] exception-count: exceptions.yaml has $$count entries (baseline $$baseline) — net-new exceptions require extending the spec"; \
		exit 1; \
	fi; \
	echo "[PASS] exception-count: $$count entries (baseline $$baseline)"

# Dead-code gate: asserts removed symbols never reappear.
no-dead:
	@if grep -rn 'InvokeOnBestNode\|RegisterNodeProvider\|NodeSelectionCriteria\|RateLimiter' --include='*.go' . 2>/dev/null | grep -v '.gomodcache' | grep -v 'tooling/arch' ; then echo "[FAIL] no-dead: found removed symbols" ; exit 1 ; fi
	@echo "[PASS] no-dead: no removed symbols found"


lint-all: lint-layering lint-invariants lint-framework-boundaries lint-no-host-exec lint-config-boundary

# Standard Go linters (golangci-lint, config: .golangci.yaml). Not yet gating:
# the tree currently has a large backlog (~7.9k issues), so CI runs this with
# continue-on-error until the high-signal buckets are driven down.
lint-go:
	golangci-lint run ./...

# Apply all auto-fixable findings (gofmt, goimports, unconvert, many revive, etc.).
lint-go-fix:
	golangci-lint run --fix ./...

lint-config:
	go run ./app/relurplint --check all
	@$(MAKE) check-config-tree-drift
	@$(MAKE) check-boot-root

test-boundary:
	go test ./framework/cfgload ./scripts/boundaryaudit -count=1 -timeout 60s

check-boot-root:
	@echo "Checking single boot root..."
	@bash scripts/check-single-boot-root.sh

check-config-tree-drift:
	@echo "Checking config tree drift..."
	@bash scripts/check-config-tree-drift.sh

generate-templates:
	go run ./cmd/gen-templates --output /tmp/relurpify-templates

check-template-drift:
	@tmpdir="$$(mktemp -d)"; \
	go run ./cmd/gen-templates --output "$$tmpdir"; \
	diff -r templates/workspace "$$tmpdir"; \
	rm -rf "$$tmpdir"; \
	echo "Templates: no drift"

test-unit: lint-config
	go test ./... -count=1 -timeout 60s

test-integ:
	go test ./... -tags integration -count=1 -timeout 120s

test-scenario:
	go test ./... -tags scenario -count=1 -timeout 180s

test-all: test-unit test-integ test-scenario
