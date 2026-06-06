.PHONY: test-unit test-integ test-scenario test-all
.PHONY: lint-config lint-config-boundary test-boundary generate-templates check-template-drift check-boot-root check-config-tree-drift
.PHONY: lint-layering lint-invariants lint-all lint-arch

# Architecture invariant gates (GP-9). Replaces the shelved scripts/boundaryaudit.
lint-arch:
	go run ./tooling/arch/cmd/archcheck

lint-layering:
	@bash scripts/lint-layering.sh

lint-invariants:
	@bash scripts/check-lint-invariants.sh

lint-framework-boundaries:
	@bash scripts/check-framework-boundaries.sh

lint-no-host-exec:
	@bash scripts/check-no-host-exec.sh

verify-agent-boundaries:
	@bash scripts/verify-agent-boundaries.sh

lint-all: lint-layering lint-invariants lint-framework-boundaries lint-no-host-exec lint-config-boundary

lint-config:
	go run ./app/relurplint --check all
	@$(MAKE) check-config-tree-drift
	@$(MAKE) check-boot-root

lint-config-boundary:
	go run ./scripts/boundaryaudit

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
