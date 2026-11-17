.PHONY: all build assets lint test check sdk-lint sdk-test run fmt

BIN ?= .tmp/bin/authcenter
TMP_ROOT := $(CURDIR)/.tmp
GO_TEST := mkdir -p $(TMP_ROOT) && TMPDIR=$(TMP_ROOT) go test

all: check build

build: assets
	mkdir -p $(TMP_ROOT)/bin
	TMPDIR=$(TMP_ROOT) go build -o $(BIN) ./cmd/authcenter

assets:
	npm run --silent build

fmt: assets
	golangci-lint fmt ./...
	npm run fmt

lint: assets
	TMPDIR=$(TMP_ROOT) golangci-lint run ./...
	npm run lint
	npm run typecheck
	npm test --silent

test: assets
	$(GO_TEST) ./...

sdk-lint:
	cd sdk/configcenter && TMPDIR=$(TMP_ROOT) golangci-lint run ./...

sdk-test:
	cd sdk/configcenter && $(GO_TEST) ./...

check: assets lint sdk-lint test sdk-test

# The binary reads configcenter connection settings from .env.local (AC_ENV_FILE).
# Runtime settings are loaded through the configcenter SDK.
run: build
	exec ./$(BIN)
