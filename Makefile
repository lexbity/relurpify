.PHONY: test-unit test-integ test-scenario test-all

test-unit:
	go test ./... -count=1 -timeout 60s

test-integ:
	go test ./... -tags integration -count=1 -timeout 120s

test-scenario:
	go test ./... -tags scenario -count=1 -timeout 180s

test-all: test-unit test-integ test-scenario
