.PHONY: test-unit test-integ test-scenario test-all
.PHONY: validate-config

validate-config:
	go run ./app/dev-agent-cli config validate
	go run ./app/relurpish validate

test-unit: validate-config
	go test ./... -count=1 -timeout 60s

test-integ:
	go test ./... -tags integration -count=1 -timeout 120s

test-scenario:
	go test ./... -tags scenario -count=1 -timeout 180s

test-all: test-unit test-integ test-scenario
