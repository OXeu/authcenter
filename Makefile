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
	golangci-lint run ./...
	npm run lint
	npm run typecheck
	npm test --silent

test: assets
	$(GO_TEST) ./...

sdk-lint:
	cd sdk/authcenter && golangci-lint run ./...

sdk-test:
	cd sdk/authcenter && $(GO_TEST) ./...

check: assets lint sdk-lint test sdk-test

# The binary loads .env.local itself (AC_ENV_FILE), without overriding
# variables exported here, so `AC_ADDR=:9000 make run` still works.
run: build
	exec ./$(BIN)
