# AMIVM: AMIVM-IRをGoソースへ変換するコンパイラ

BINARY := amivm
GO     := go

.PHONY: all build test fmt vet tidy clean help

all: build ## デフォルトターゲット(ビルドのみ)

build: ## amivmバイナリをビルドする
	$(GO) build -o $(BINARY) main.go

test: build ## test_ir/配下の全IRファイルを変換できるか検証する(生成物は一時ディレクトリに書き出し、リポジトリは汚さない)
	@set -e; \
	tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	for ir in test_ir/*.ir; do \
		name=$$(basename "$$ir" .ir); \
		echo "== $$ir =="; \
		./$(BINARY) "$$ir" -o "$$tmp/$$name.go" -v; \
	done

fmt: ## main.goをgoimportsで整形する
	goimports -w main.go

vet: ## go vetで静的検査する
	$(GO) vet main.go

tidy: ## go.mod/go.sumを整理する
	$(GO) mod tidy

clean: ## ビルド成果物・テスト時に取り違えて生成された.goファイルを削除する
	rm -f $(BINARY)
	rm -f test_ir/*.go

help: ## 使えるターゲット一覧を表示する
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2}'
