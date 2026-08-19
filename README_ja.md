# AMIVM

AMIVMは、独自の中間表現(**AMIVM-IR**)をGoのソースコードに変換するコンパイル基盤です。Goの並行処理機構(goroutine・channel)は後付けではなく、IRの段階から直接組み込まれているのが特徴です。

> [English README is here](README.md)

## ステータス

現状は**試作段階の実装**です。コンパイラ本体は`main.go`1ファイルにまとまっており、対象言語からAMIVM-IRへの変換を担う「フロントエンド」は意図的にこのリポジトリのスコープ外としています([パイプライン](#パイプライン)参照)。

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
go build -o amivm main.go
```

または同梱の`Makefile`を使う(`make help`で全ターゲット一覧を表示。`test`は`test_ir/`配下の全サンプルをコンパイラに通す)。

```sh
make build
```

## 使い方

```
amivm <IRファイルパス> [-o <出力ファイルパス>] [-v]
```

| オプション | 説明 |
|---|---|
| `-o <出力ファイルパス>` | 生成したGoファイルの出力先。省略時は`<IRファイルパス>`の拡張子を`.go`に置き換えたパス(拡張子が無ければ`.go`を付け足したパス)になる。 |
| `-v` | 処理内容(元のIR・未使用変数の自己修復ログ・最終的な生成コード・成功メッセージ)を表示する。付けない場合、成功時は何も出力しない(エラーは`-v`の有無に関わらず常に出力される)。 |

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

全命令を網羅した実行可能なサンプルを、カテゴリ別(変数定義・通常演算・ビット演算・シフト演算・論理演算・比較演算・文字列操作・ポインタ・配列とGOTOによるループ・関数とDEFER・goroutine/channel/SEL・スライス・構造体・map・クロージャー・Goメソッド呼び出し)に分けて[`test_ir/`](test_ir/)に置いています。

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
- **配列・制御フロー**: `ASET`, `AGET`, `LABEL`, `GOTO`, `IF`
- **関数**: `FUNC`/`ENDFUNC`, `RET`, `CALL`, `DEFER`, `SPAWN`
- **チャネル・`select`**: `CHTYPE`, `CHMAKE`, `CHSEND`, `CHRECV`, `SEL`/`ENDSEL`, `CASESEND`, `CASERECV`, `DEFAULT`
- **スライス**: `SLTYPE`, `SLMAKE`, `SLICE`
- **構造体**: `STTYPE`/`ENDSTTYPE`, `FIELD`, `FSET`, `FGET`
- **map**: `MPTYPE`, `MPMAKE`, `MSET`, `MGET`
- **クロージャー**: `FNTYPE`, `CLOS`/`ENDCLOS`

メソッド呼び出し(例: `file.Close()`)は、`FNTYPE`でメソッドの関数型を宣言し、`FGET`で構造体変数からメソッド値を取り出し、その値を呼び出すという形で表現します。

**唯一の正確な仕様は[`amivm_spec.md`](amivm_spec.md)です。** 本READMEを含む他のドキュメントと矛盾する場合は`amivm_spec.md`を優先してください。同じ仕様を設計判断の理由まで含めてより読みやすく解説したものが[`amivm_instruction_spec.md`](amivm_instruction_spec.md)、コンパイラ内部の実装(トークナイズ・`Kind`/`Category`体系・AST組み立て・未使用変数の自己修復処理など)の解説が[`amivm_code_design.md`](amivm_code_design.md)にあります。

## 制約

- `FUNC`はトップレベルのみに置け、関数のネストはできません。`STTYPE`・`CLOS`・`SEL`も同様にネスト不可です。
- 配列は1次元固定長のみです。多次元配列は、(未実装の)フロントエンド側でAMIVM-IRに渡す前に1次元へ展開する前提です。
- 意味的な正しさ(型整合性・未定義識別子・メソッドの存在チェックなど)は全て`go/types`に委ねています。AMIVM自身が保証するのは、構文的に妥当なGoコードを出力することだけです。

## リポジトリ構成

```
main.go                     コンパイラ本体(トークナイズ→分類→パース→ast.File組み立て→Go出力)
Makefile                    ビルド・テスト・クリーンアップ用タスク(`make help`で一覧表示)
amivm_spec.md               唯一の正確な仕様(プロジェクト概要+IR仕様の全体)
amivm_instruction_spec.md   amivm_spec.mdの解説版(設計判断の理由まで含む)
amivm_code_design.md        main.goの内部設計メモ
test_ir/                    命令カテゴリ別のサンプルIR
CLAUDE.md                   AIによる開発支援のためのプロジェクト規約
LICENSE                     MIT
```

## ライセンス

[MIT](LICENSE)
