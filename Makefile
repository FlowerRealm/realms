SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

ROOT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
TOOLS_BIN := $(ROOT_DIR)/.tmp/bin
AIR := $(TOOLS_BIN)/air

.PHONY: help tools dev test fmt tidy

help:
	@echo "Targets:"
	@echo "  make tools   安装开发工具（air，安装到 .tmp/bin）"
	@echo "  make dev     开发热重载（仅在配置 MySQL 时自动启动 MySQL docker 容器）"
	@echo "  make test    运行测试"
	@echo "  make fmt     gofmt（按包目录）"
	@echo "  make tidy    go mod tidy"

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
