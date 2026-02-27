SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

ROOT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
TOOLS_BIN := $(ROOT_DIR)/.tmp/bin
AIR := $(TOOLS_BIN)/air

EXE :=
ifeq ($(OS),Windows_NT)
  EXE := .exe
endif

.PHONY: help tools dev test ci ci-real fmt tidy app-dev app-dist
.PHONY: app-set-key

help:
	@echo "Targets:"
	@echo "  make tools   安装开发工具（air，安装到 .tmp/bin）"
	@echo "  make dev     开发热重载（后端 air + 前端 dist watch；本地:8080；自动启动 docker cli-runner；不自动启动 MySQL）"
	@echo "  make test    运行测试"
	@echo "  make ci      运行 CI 检查集（本地/CI 同口径；如设置 REALMS_CI_* 则默认跑 real upstream）"
	@echo "  make ci-real 运行真实上游集成回归（需要 REALMS_CI_*）"
	@echo "  make app-dev  App 开发运行（浏览器 + 端口；personal 模式默认 :8080）"
	@echo "  make app-dist App 打包（当前平台二进制）"
	@echo "  make fmt     gofmt（按包目录）"
	@echo "  make tidy    go mod tidy"

tools: $(AIR)

$(AIR):
	@mkdir -p "$(TOOLS_BIN)"
	@echo ">> installing air -> $(TOOLS_BIN)"
	@GOBIN="$(TOOLS_BIN)" go install github.com/air-verse/air@latest

dev: tools
	@PATH="$(TOOLS_BIN):$$PATH" bash ./scripts/dev.sh

app-dev:
	@npm --prefix "web" install
	@npm --prefix "web" run build:personal
	@go run -tags embed_web_personal ./cmd/realms-app

app-dist:
	@npm --prefix "web" install
	@npm --prefix "web" run build:personal
	@mkdir -p "dist"
	@go build -tags embed_web_personal -ldflags "-X realms/internal/version.Version=$(VERSION) -X realms/internal/version.Date=$(DATE)" -o "dist/realms-app$(EXE)" ./cmd/realms-app

app-set-key:
	@if [[ -z "$${KEY:-}" ]]; then echo "KEY is required (example: make app-set-key KEY='sk_...')"; exit 2; fi
	@go run ./cmd/realms-app --set --key "$${KEY}"

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
