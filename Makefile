.PHONY: test-unit test-integ test-scenario test-all
.PHONY: validate-config lint-config-boundary test-boundary generate-templates check-template-drift check-boot-root check-config-tree-drift

validate-config:
	go run ./app/relurpish validate-config
	@$(MAKE) check-config-tree-drift
	@$(MAKE) check-boot-root

lint-config-boundary:
	@echo "Checking config boundary..."
	@! rg -n 'os\.(Getenv|Environ|LookupEnv|Setenv)\(' --glob '*.go' . | grep -v '_test.go' | grep -v 'framework/cfgload/' | grep -v 'framework/runtimeenv/' >/dev/null \
		|| (echo "Config boundary: FAIL — env access outside framework/cfgload and framework/runtimeenv"; exit 1)
	@! rg -n 'relurpify_cfg' --glob '*.go' . | grep -v '_test.go' | rg 'os\.ReadFile|os\.ReadDir|os\.WriteFile|os\.OpenFile' >/dev/null \
		|| (echo "Config boundary: FAIL — config path access outside framework/cfgload"; exit 1)
	@echo "Config boundary: PASS"

test-boundary:
	go test ./framework/configcheck ./framework/cfgload -count=1 -timeout 60s

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

test-unit: validate-config
	go test ./... -count=1 -timeout 60s

test-integ:
	go test ./... -tags integration -count=1 -timeout 120s

test-scenario:
	go test ./... -tags scenario -count=1 -timeout 180s

test-all: test-unit test-integ test-scenario
