.PHONY: all build assets lint test check sdk-lint sdk-test run fmt

BIN ?= .tmp/bin/configcenter
TMP_ROOT := $(CURDIR)/.tmp
GO_TEST := mkdir -p $(TMP_ROOT) && TMPDIR=$(TMP_ROOT) go test

all: check build

build: assets
	mkdir -p $(TMP_ROOT)/bin
	TMPDIR=$(TMP_ROOT) go build -o $(BIN) ./cmd/configcenter

assets:
	npm run --silent build

fmt: assets
	golangci-lint fmt ./...
	npm run fmt

lint: assets
	golangci-lint run ./...
	npm run lint
	npm run typecheck
	npm test --silent

test: assets
	$(GO_TEST) ./...

sdk-lint:
	cd sdk/configcenter && golangci-lint run ./...

sdk-test:
	cd sdk/configcenter && $(GO_TEST) ./...

check: assets lint sdk-lint test sdk-test

# The binary loads .env.local itself (CC_ENV_FILE), without overriding
# variables exported here, so `CC_ADDR=:9000 make run` still works.
run: build
	exec ./$(BIN)
