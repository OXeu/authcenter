PROJECT := authcenter
COMMON_DIR ?= ../center-common
.DEFAULT_GOAL := all
.NOTPARALLEL:
.PHONY: common common-local bootstrap all build assets lint test check sdk-lint sdk-test run fmt scf-package

common:
	node tools/common.ts

common-local:
	node tools/common.ts --local "$(COMMON_DIR)"

bootstrap: common
	cd .tmp/common && npm ci
	npm ci

all build assets lint test check sdk-lint sdk-test run fmt: common
	$(MAKE) --no-print-directory -f .tmp/common/config/project.mk PROJECT=$(PROJECT) $@

scf-package:
	node tools/common.ts --locked
	$(MAKE) --no-print-directory -f .tmp/common/config/project.mk PROJECT=$(PROJECT) $@
