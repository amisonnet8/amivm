# AMIVM: AMIVM-IRをGoソースへ変換するコンパイラ

BINARY := amivm
PKG    := ./cmd/amivm
GO     := go

.PHONY: all build install unit-test test fmt vet tidy clean help

all: build ## デフォルトターゲット(ビルドのみ)

build: ## amivmバイナリをビルドする
	$(GO) build -o $(BINARY) $(PKG)

install: ## amivmバイナリをGOBIN($GOPATH/bin)へインストールする(xxlang等の外部プロジェクトから使う場合はこちら)
	$(GO) install $(PKG)

unit-test: ## go testでcmd/amivm配下のユニットテストを実行する
	$(GO) test ./...

test: build unit-test ## unit-test + examples/配下の全IRファイルを変換できるか検証する(生成物は一時ディレクトリに書き出し、リポジトリは汚さない)
	@set -e; \
	tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	for ir in examples/*.ir; do \
		name=$$(basename "$$ir" .ir); \
		echo "== $$ir =="; \
		./$(BINARY) "$$ir" -o "$$tmp/$$name.go" -v; \
	done

fmt: ## *.goをgoimportsで整形する
	goimports -w $(PKG)

vet: ## go vetで静的検査する
	$(GO) vet $(PKG)

tidy: ## go.mod/go.sumを整理する
	$(GO) mod tidy

clean: ## ビルド成果物・テスト時に取り違えて生成された.goファイルを削除する
	rm -f $(BINARY)
	rm -f examples/*.go

help: ## 使えるターゲット一覧を表示する
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2}'
