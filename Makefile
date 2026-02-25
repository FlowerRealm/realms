SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

ROOT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
TOOLS_BIN := $(ROOT_DIR)/.tmp/bin
AIR := $(TOOLS_BIN)/air

.PHONY: help tools dev test ci ci-real fmt tidy desktop-dev desktop-dist

help:
	@echo "Targets:"
	@echo "  make tools   安装开发工具（air，安装到 .tmp/bin）"
	@echo "  make dev     开发热重载（后端 air + 前端 dist watch；本地:8080；自动启动 docker cli-runner；不自动启动 MySQL）"
	@echo "  make test    运行测试"
	@echo "  make ci      运行 CI 检查集（本地/CI 同口径；如设置 REALMS_CI_* 则默认跑 real upstream）"
	@echo "  make ci-real 运行真实上游集成回归（需要 REALMS_CI_*）"
	@echo "  make desktop-dev  桌面版开发运行（Electron，自用模式；固定 127.0.0.1:8080）"
	@echo "  make desktop-dist 桌面版打包（当前平台安装包）"
	@echo "  make fmt     gofmt（按包目录）"
	@echo "  make tidy    go mod tidy"

tools: $(AIR)

$(AIR):
	@mkdir -p "$(TOOLS_BIN)"
	@echo ">> installing air -> $(TOOLS_BIN)"
	@GOBIN="$(TOOLS_BIN)" go install github.com/air-verse/air@latest

dev: tools
	@PATH="$(TOOLS_BIN):$$PATH" bash ./scripts/dev.sh

desktop-dev:
	@npm --prefix "web" install
	@npm --prefix "web" run build:self
	@npm --prefix "desktop" install
	@npm --prefix "desktop" run dev

desktop-dist:
	@npm --prefix "web" install
	@npm --prefix "desktop" install
	@npm --prefix "desktop" run dist

test:
	go test ./...

ci:
	bash "./scripts/ci.sh"

ci-real:
	bash "./scripts/ci-real.sh"

fmt:
	gofmt -w $$(go list -f '{{.Dir}}' ./...)

tidy:
	go mod tidy
