.PHONY: test-unit test-integ test-scenario test-all
.PHONY: lint-config lint-config-boundary test-boundary generate-templates check-template-drift check-boot-root check-config-tree-drift
.PHONY: lint-layering lint-invariants lint-all lint-arch
.PHONY: domain-check domain-cycles no-bucket no-dead

# Architecture invariant gates (GP-9). Replaces the shelved scripts/boundaryaudit.
lint-arch:
	go run ./tooling/arch/cmd/archcheck

# Domain DAG direction checker (§2.1). Warn-mode: reports violations, exits 0.
domain-check:
	go run ./tooling/arch/cmd/domaincheck -mode=warn -check=direction

# Domain-level cycle reporter. Lists every mutual (cyclic) domain pair.
domain-cycles:
	go run ./tooling/arch/cmd/domaincheck -mode=warn -check=cycles

# No-bucket guard: flags any type-only package imported by ≥3 domains.
no-bucket:
	go run ./tooling/arch/cmd/domaincheck -mode=warn -check=nobucket

# Dead-code gate: asserts removed symbols never reappear.
no-dead:
	@if grep -rn 'InvokeOnBestNode\|RegisterNodeProvider\|NodeSelectionCriteria\|RateLimiter' --include='*.go' . 2>/dev/null | grep -v '.gomodcache' | grep -v 'tooling/arch' ; then echo "[FAIL] no-dead: found removed symbols" ; exit 1 ; fi
	@if grep -rni 'mcp.*providerkind\|ProviderKindMCP\|named/factory' --include='*.go' capability/ agents/ governance/ 2>/dev/null | grep -v '.gomodcache' ; then echo "[FAIL] no-dead: found MCP or named/factory references" ; exit 1 ; fi
	@if grep -rn '^[[:space:]]*Config[[:space:]]\+map\[string\]any' --include='*.go' agents/ 2>/dev/null ; then echo "[FAIL] no-dead: found Config map[string]any in agents/" ; exit 1 ; fi
	@echo "[PASS] no-dead: no removed symbols found"


lint-all: lint-layering lint-invariants lint-framework-boundaries lint-no-host-exec lint-config-boundary

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
