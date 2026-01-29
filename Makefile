SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

ROOT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
TOOLS_BIN := $(ROOT_DIR)/.tmp/bin
AIR := $(TOOLS_BIN)/air

.PHONY: help tools dev test fmt tidy release-artifacts deb

help:
	@echo "Targets:"
	@echo "  make tools   安装开发工具（air，安装到 .tmp/bin）"
	@echo "  make dev     开发热重载（本地:8080 正常模式；不自动启动 Docker）"
	@echo "  make test    运行测试"
	@echo "  make fmt     gofmt（按包目录）"
	@echo "  make tidy    go mod tidy"
	@echo "  make release-artifacts VERSION=vX.Y.Z   构建发布产物（dist/）"
	@echo "  make deb VERSION=vX.Y.Z ARCH=amd64      构建 .deb（dist/）"

tools: $(AIR)

$(AIR):
	@mkdir -p "$(TOOLS_BIN)"
	@echo ">> installing air -> $(TOOLS_BIN)"
	@GOBIN="$(TOOLS_BIN)" go install github.com/air-verse/air@latest

dev: tools
	@PATH="$(TOOLS_BIN):$$PATH" bash ./scripts/dev.sh

test:
	go test ./...

fmt:
	gofmt -w $$(go list -f '{{.Dir}}' ./...)

tidy:
	go mod tidy

release-artifacts:
	bash "./scripts/build-release.sh" "$(VERSION)" "dist"

deb:
	bash "./scripts/build-deb.sh" "$(VERSION)" "$(ARCH)" "dist"
