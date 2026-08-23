# AMIVM

[![test](https://github.com/amisonnet8/amivm/actions/workflows/test.yml/badge.svg)](https://github.com/amisonnet8/amivm/actions/workflows/test.yml)

AMIVMは、独自の中間表現(**AMIVM-IR**)をGoのソースコードに変換するコンパイル基盤です。Goの並行処理機構(goroutine・channel)は後付けではなく、IRの段階から直接組み込まれているのが特徴です。

> [English README is here](README.md)

## ステータス

現状は**試作段階の実装**です。コンパイラ本体は単一のGoパッケージ(サブディレクトリなし)で、役割ごとにいくつかのファイルへ分割されています。対象言語からAMIVM-IRへの変換を担う「フロントエンド」は意図的にこのリポジトリのスコープ外としています([パイプライン](#パイプライン)参照)。

## パイプライン

```
対象言語
  ↓(フロントエンド ※このリポジトリの対象外)
AMIVM-IR
  ↓ AMIVM(このリポジトリ)
Goソースコード
  ↓ go build
実行ファイル
```

AMIVM自身の責務はGoソースファイルを出力するところまでです。それを実行ファイルにする工程(単純な`go build`)は別の後続ステップであり、`amivm`コマンド自体は行いません。

## 動作要件

- Go([`go.mod`](go.mod)記載のバージョン。現在は1.26.5)。ビルド時にimport解決用の`golang.org/x/tools/imports`を取得します。

## ビルド

```sh
go build -o amivm ./cmd/amivm
```

または同梱の`Makefile`を使う(`make help`で全ターゲット一覧を表示。`test`は`test_ir/`配下の全サンプルをコンパイラに通す)。

```sh
make build
```

このリポジトリをクローンせずに、リリース済みの`amivm`を最新版で入れたい場合。

```sh
go install github.com/amisonnet8/amivm/cmd/amivm@latest
```

`$GOBIN`(未設定なら`$GOPATH/bin`)に配置されるので、そのディレクトリが`PATH`に通っていることを確認する。

開発中の別プロジェクト(例: AMIVM-IRを出力するフロントエンド言語実装)から`amivm`を使いたい場合は、バイナリをコピーして回るのではなく、ローカルにクローンした本リポジトリから`PATH`の通った場所にインストールする。

```sh
make install   # go install ./cmd/amivm — $GOBIN(未設定なら$GOPATH/bin)に配置される
```

`make install`を実行し直すだけで、相手プロジェクト側は常に最新の`amivm`を使える(2リポジトリ間でパスを持ち合う必要がない)。`amivm`を最終的にエンドユーザー向けにどこへ配置するか(例: `/usr/local/bin`)は、そのダウンストリームのプロジェクト側が決めることであり、本リポジトリの関心事ではない。

## 使い方

```
amivm <IRファイルパス> [-o|--output <出力ファイルパス>] [-v|--verbose] [-i|--import <名前>=<importパス>]...
```

| オプション | 説明 |
|---|---|
| `-o <出力ファイルパス>`, `--output <出力ファイルパス>` | 生成したGoファイルの出力先。省略時は`<IRファイルパス>`の拡張子を`.go`に置き換えたパス(拡張子が無ければ`.go`を付け足したパス)になる。 |
| `-v`, `--verbose` | 処理内容(元のIR・未使用変数の自己修復ログ・最終的な生成コード・成功メッセージ)を表示する。付けない場合、成功時は何も出力しない(エラーは`-v`の有無に関わらず常に出力される)。 |
| `-i <名前>=<importパス>`, `--import <名前>=<importパス>` | 繰り返し指定可能。指定した名前・パスの組で`import <名前> "<importパス>"`という明示的なimportを生成コードにあらかじめ追加する。`?<名前>.Func`という呼び出しがそのimportを直接使うようになり、goimportsが裸の識別子からimportパスを推測する(標準ライブラリや既に参照済みのパッケージ以外では信頼できない)処理を経由しなくて済む。下流のプロジェクト(AMIVM-IRを出力するフロントエンド言語実装等)が用意した独自のGoライブラリを呼びたい場合に使う。生成コード内で結局使われなかった名前は自動的に取り除かれるため、同じ`-i`/`--import`の組を全てのIRファイルに対して使い回しても問題ない。 |

## 例

```
FUNC	!main	:
	CALL	:	?fmt.Println	"Hello, AMIVM!"
	RET
ENDFUNC
```

```sh
$ amivm hello.ir -v
=== IR ===
...
=== 最終生成コード ===
package main

import "fmt"

func main() {
	fmt.Println("Hello, AMIVM!")
	return
}

生成成功: hello.go
$ go run hello.go
Hello, AMIVM!
```

全命令を網羅した実行可能なサンプルを、カテゴリ別(変数定義・通常演算・ビット演算・シフト演算・論理演算・比較演算・文字列操作・ポインタ・配列とGOTOによるループ・関数とDEFER・goroutine/channel/SEL・スライス・構造体・map・クロージャー・Goメソッド呼び出し・構造化されたIF/LOOP制御フロー・型アサーション)に分けて[`test_ir/`](test_ir/)に置いています。

## IR言語の概要

全ての識別子は先頭1文字のプレフィックスで種別が確定します。

| プレフィックス | 意味 |
|---|---|
| `$` | 関数引数(パラメータ) |
| `&` | クロージャー引数 |
| `%` | 関数内変数(ローカル変数) |
| `@` | パッケージレベル変数 |
| `^` | 型名 |
| `>` | 構造体フィールド名 |
| `!` | AMIVM内定義関数名 |
| `?` | Go関数名 |
| `#` | ラベル名 |

命令は大まかに次のように分類されます。

- **変数・代入**: `VAR`, `GVAR`, `SET`
- **算術・ビット演算・シフト・論理・比較**: `ADD` `SUB` `MUL` `DIV` `MOD` ・ `BAND` `BOR` `BXOR` `BCLEAR` `BNOT` ・ `SHL` `SHR` ・ `AND` `OR` `NOT` ・ `EQ` `NEQ` `LT` `LTE` `GT` `GTE`
- **文字列**: `CONCAT`, `SLICE`
- **ポインタ**: `ADDR`, `PGET`, `PSET`
- **配列**: `ASET`, `AGET`
- **ラベル・goto**: `LABEL`, `GOTO`
- **条件分岐**: `IF`/`ELIF`/`ELSE`/`ENDIF`
- **ループ**: `LOOP`/`BREAK`/`CONTINUE`/`ENDLOOP`
- **型アサーション**: `ASSERT`
- **関数**: `FUNC`/`ENDFUNC`, `RET`, `CALL`, `DEFER`, `SPAWN`
- **チャネル・`select`**: `CHTYPE`, `CHMAKE`, `CHSEND`, `CHRECV`, `SEL`/`ENDSEL`, `CASESEND`, `CASERECV`, `DEFAULT`
- **スライス**: `SLTYPE`, `SLMAKE`, `SLICE`
- **構造体**: `STTYPE`/`ENDSTTYPE`, `FIELD`, `FSET`, `FGET`
- **map**: `MPTYPE`, `MPMAKE`, `MSET`, `MGET`, `MPKEYS`
- **クロージャー**: `FNTYPE`, `CLOS`/`ENDCLOS`

メソッド呼び出し(例: `file.Close()`)は、`FNTYPE`でメソッドの関数型を宣言し、`FGET`で構造体変数からメソッド値を取り出し、その値を呼び出すという形で表現します。

**唯一の正確な仕様は[`docs/amivm_spec.md`](docs/amivm_spec.md)です。** 本READMEを含む他のドキュメントと矛盾する場合は`amivm_spec.md`を優先してください。同じ仕様を設計判断の理由まで含めてより読みやすく解説したものが[`docs/amivm_instruction_spec.md`](docs/amivm_instruction_spec.md)です。コンパイラ内部の実装(ファイル構成・トークナイズ・`Kind`/`Category`体系・AST組み立て・未使用変数の自己修復処理など)を知りたい場合は、コメントを充実させてある`cmd/amivm/`配下のソースコードを直接参照してください。

## 独自のGoコードを呼ぶ

AMIVM-IRの上に構築される下流のプロジェクト(言語のフロントエンド実装等)は、標準ライブラリだけでなく、自分で書いたランタイムライブラリを生成プログラムから呼びたいことが多いはずです。これは`fmt.Println`等で既に使っている`?pkg.Func` / `CALL`という同じ仕組みでそのまま実現できます(「独自コード」だからといってIR側に特別な構文があるわけではありません)。ただし押さえておくべき点が2つあります。

1. **自分の関数を普通のGoパッケージとして用意する。** プロジェクトに合ったモジュール構成であれば何でもよく、どこかに公開する必要もありません。プライベートモジュール(`GOPRIVATE`でプライベートなVCSホストから取得する、あるいはモノレポ構成で`replace`ディレクティブでローカル参照する等)でも、公開パッケージと全く同じようにamivmから見えます。
2. **amivmが出力先とするディレクトリはGoモジュールである必要がある**(そのディレクトリ自体か、その親のいずれかに`go.mod`が存在すること)。別packageへの呼び出し(生成ファイルの隣に置いた手書きファイルであれ、同一モジュール内の別packageであれ)の型チェックは`golang.org/x/tools/go/packages`に依存しており、これは`go build`自体と同様、モジュールとして解決できる文脈が無いと単一ファイルの外を見てくれません。
3. **amivmがまだ見たことのないパッケージを参照する場合は`-i name=importパス`を渡す。** `xxrt.Helper`のような裸の識別子から正しいimportを自動挿入するのはgoimportsの仕事ですが、これが信頼できるのは標準ライブラリと既にどこかで参照済みのパッケージだけです。導入したばかりのパッケージに対しては、importの挿入に失敗する、あるいは誤ったパスを挿入することすらあります。明示的に渡せば、この推測自体を完全に回避できます。

例えば、次のような小さなランタイムパッケージがあるとして:

```go
// xxrt/xxrt.go — 呼び出し側モジュールに属する(あるいはその依存である)package xxrt
package xxrt

func Helper(a, b int) int { return a*10 + b }
```

次のIRに対して:

```
FUNC	!main	:
	VAR	%r	^int
	CALL	%r	:	?xxrt.Helper	1	2
	CALL	:	?fmt.Println	%r
	RET
ENDFUNC
```

(`yourmodule`の中で、または`-o`が`yourmodule`配下を指すようにして)`amivm hello.ir -o hello.go -i xxrt=yourmodule/xxrt`を実行すると、`import xxrt "yourmodule/xxrt"`が既に入った`hello.go`が生成され、そのまま`go build`できます。

**メソッド呼び出し**(例: `file.Close()`、あるいは自分の型に定義したメソッド)も、レシーバーの型が標準ライブラリのものか自分で定義したものかに関わらず同じ手順です。`FNTYPE`でメソッドの関数型を宣言し、`FGET`で構造体変数からメソッド値を取り出し、その値を呼び出します。`(*os.File).Close`を例にした実例が[`test_ir/16_method_call.ir`](test_ir/16_method_call.ir)にあります。

## 制約

- `FUNC`はトップレベルのみに置け、関数のネストはできません。`STTYPE`も同様にネスト不可です。`IF`・`LOOP`・`CLOS`・`SEL`はいずれもネストでき、互いの中にも書けます。
- 配列は1次元固定長のみです。多次元配列は、(未実装の)フロントエンド側でAMIVM-IRに渡す前に1次元へ展開する前提です。
- 意味的な正しさ(型整合性・未定義識別子・メソッドの存在チェックなど)は全て`go/types`に委ねています。AMIVM自身が保証するのは、構文的に妥当なGoコードを出力することだけです。

## リポジトリ構成

```
cmd/amivm/
  token.go                   トークナイズ+分類(Kind体系)
  astbuild.go                命名規則とAtom→ast.Exprの組み立て
  category.go                オペランドカテゴリ(許容Kind集合)と検証
  parse_stmt.go              1行完結命令(VAR/SET/CALL等)のパース
  parse_block.go             ブロック構造(FUNC/SEL/CLOS/STTYPE、TYPE系宣言)のパース
  program.go                 トップレベルの組み立て(buildProgram)
  compile.go                 未使用変数の自己修復+Goソース出力パイプライン
  main.go                    エントリポイント(CLI引数解釈・main)
docs/
  amivm_spec.md               唯一の正確な仕様(プロジェクト概要+IR仕様の全体)
  amivm_instruction_spec.md   amivm_spec.mdの解説版(設計判断の理由まで含む)
Makefile                    ビルド・テスト・クリーンアップ用タスク(`make help`で一覧表示)
test_ir/                    命令カテゴリ別のサンプルIR
.github/workflows/test.yml  CI: push/PR時にgofmt/go vet/go test/make testを実行
CLAUDE.md                   AIによる開発支援のためのプロジェクト規約
LICENSE                     MIT
```

## ライセンス

[MIT](LICENSE)
