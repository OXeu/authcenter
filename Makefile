.PHONY: all build assets lint test check sdk-lint sdk-test run fmt

BIN ?= bin/configcenter
TMP_ROOT := $(CURDIR)/.tmp
GO_TEST := mkdir -p $(TMP_ROOT) && TMPDIR=$(TMP_ROOT) go test

all: check build

build: assets
	go build -o $(BIN) ./cmd/configcenter

assets:
	npm run --silent build

fmt:
	golangci-lint fmt ./...
	npx eslint --fix 'internal/server/templates/**/*.html' web/ui

lint:
	golangci-lint run ./...
	npx eslint web/ui 'internal/server/templates/**/*.html'
	npx markuplint 'internal/server/templates/**/*.html'
	npx tsc -p jsconfig.json
	npm test --silent

test:
	$(GO_TEST) ./...

sdk-lint:
	cd sdk/configcenter && golangci-lint run ./...

sdk-test:
	cd sdk/configcenter && $(GO_TEST) ./...

check: lint sdk-lint test sdk-test

# The binary loads .env.local itself (CC_ENV_FILE), without overriding
# variables exported here, so `CC_ADDR=:9000 make run` still works.
run: build
	exec ./$(BIN)
