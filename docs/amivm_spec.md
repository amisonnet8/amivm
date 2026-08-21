# AMIVM 仕様書

元は`PJ.txt`(企画メモ)と`IR.txt`(命令セット仕様)という2つの箇条書きファイルだったものを1つに統合し、体系立てて構造化したもの。統合時点では内容はどちらの原本にも忠実で、新たな設計判断や経緯の解説は加えていない(`PJ.txt`/`IR.txt`は統合後に削除済みで、本ドキュメントがその後継)。

> **唯一の正確な仕様は本ドキュメント。** 他のドキュメントと矛盾する場合は本ドキュメントを優先する。設計判断の経緯や変更の理由を知りたい場合は`amivm_instruction_spec.md`を、コンパイラ本体の内部実装を知りたい場合は`amivm_code_design.md`を参照。

## 目次

1. [概要](#1-概要)
2. [制約・前提条件](#2-制約前提条件)
3. [識別子のプレフィックス](#3-識別子のプレフィックス)
4. [命令一覧](#4-命令一覧)
5. [オペランドカテゴリ](#5-オペランドカテゴリ)
6. [トークンの形状分類(Kind)](#6-トークンの形状分類kind)

## 1. 概要

AMIVM(仮称)は、独自の中間言語(AMIVM-IR)をGoコードに変換するコンパイル基盤である。

```
対象言語
  ↓ (フロントエンド。AIに設計してもらう予定・本仕様の対象外)
中間言語(AMIVM-IR)
  ↓ AMIVM
Goコード
  ↓ (go build)
実行ファイル
```

**特徴**: Goの並行処理(goroutine/channel)を中間言語に直接取り込んでいる。

## 2. 制約・前提条件

### 2.1 構造上の制約

- `FUNC`はトップレベルのみに置ける(関数のネスト不可)
- `FUNC`・`STTYPE`・`SEL`はネスト不可。`CLOS`のみ例外で、`CLOS`本体の中にさらに`CLOS`をネストできる。ネストの深さは`FUNC`直下を1として数え、クロージャー引数`&L-N`の階層番号`L`に対応する
- 配列は1次元固定長のみ
- 多次元配列はAMIVM-IR自体では表現しない。多次元配列はフロントエンド側で1次元に展開する

### 2.2 型定義の前提

- スライス・マップ・構造体・クロージャー(関数型)は、対応する`TYPE`系命令(`SLTYPE`/`MPTYPE`/`STTYPE`/`FNTYPE`)で型を定義してから使う
- `FNTYPE`でレシーバー込みの関数型を定義 → `FGET`でメソッドを値として取得、という手順で`file.Close()`のようなメソッド呼び出しができる

### 2.3 トークン記法

- トークンの区切り文字は**タブ**
- 行頭の空白カラム群(インデント用の連続タブ)は無視する
- カラムの中身はそのまま使う。スペースのトリムなどはしない
- `//`で始まる行はコメント行として無視する

## 3. 識別子のプレフィックス

全ての識別子は先頭の記号で種別が決まる。

| 記号 | 意味 |
|---|---|
| `$` | 関数引数 |
| `&` | クロージャー引数(`&N`は自分がいる`CLOS`階層のN番目、`&L-N`は階層`L`のN番目を明示的に指定) |
| `%` | 関数内変数名 |
| `@` | 関数外変数名 |
| `^` | 型名 |
| `>` | 構造体フィールド名 |
| `!` | vm定義関数名 |
| `?` | Go関数名 |
| `#` | ラベル名 |

## 4. 命令一覧

### 4.1 変数宣言

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `VAR local type1` | `var local type1` | |
| `GVAR global type1` | `var global type1` | 関数外 |

### 4.2 代入・ポインタ・配列アクセス

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `SET single1 value1` | `single1 = value1` | |
| `ASET single1 whole value1` | `single1[whole] = value1` | |
| `AGET single1 variable whole` | `single1 = variable[whole]` | |
| `PSET single1 value1` | `*single1 = value1` | |
| `PGET single1 variable` | `single1 = *variable` | |
| `ADDR single1 variable (point)` | `point`無し: `single1 = &variable` / `point`が`>xxx_123`: `single1 = &variable.point` / `point`が`>xxx_123`ではない: `single1 = &variable[point]` | `&variable[point]`はスライス/配列専用。mapに使うとgo/typesエラーになる(想定通りの挙動) |

### 4.3 算術演算

| 命令 | 生成されるGoコード |
|---|---|
| `ADD single1 number1 number2` | `single1 = number1 + number2` |
| `SUB single1 number1 number2` | `single1 = number1 - number2` |
| `MUL single1 number1 number2` | `single1 = number1 * number2` |
| `DIV single1 number1 number2` | `single1 = number1 / number2` |
| `MOD single1 integer1 integer2` | `single1 = integer1 % integer2` |

### 4.4 ビット演算

| 命令 | 生成されるGoコード |
|---|---|
| `BAND single1 integer1 integer2` | `single1 = integer1 & integer2` |
| `BOR single1 integer1 integer2` | `single1 = integer1 \| integer2` |
| `BXOR single1 integer1 integer2` | `single1 = integer1 ^ integer2` |
| `BCLEAR single1 integer1 integer2` | `single1 = integer1 &^ integer2` |
| `BNOT single1 integer1` | `single1 = ^integer1` |

### 4.5 シフト演算

| 命令 | 生成されるGoコード |
|---|---|
| `SHL single1 integer1 whole` | `single1 = integer1 << whole` |
| `SHR single1 integer1 whole` | `single1 = integer1 >> whole` |

### 4.6 論理演算

| 命令 | 生成されるGoコード |
|---|---|
| `AND single1 boolean1 boolean2` | `single1 = boolean1 && boolean2` |
| `OR single1 boolean1 boolean2` | `single1 = boolean1 \|\| boolean2` |
| `NOT single1 boolean1` | `single1 = !boolean1` |

### 4.7 比較演算

| 命令 | 生成されるGoコード |
|---|---|
| `EQ single1 value1 value2` | `single1 = value1 == value2` |
| `NEQ single1 value1 value2` | `single1 = value1 != value2` |
| `LT single1 ordered1 ordered2` | `single1 = ordered1 < ordered2` |
| `LTE single1 ordered1 ordered2` | `single1 = ordered1 <= ordered2` |
| `GT single1 ordered1 ordered2` | `single1 = ordered1 > ordered2` |
| `GTE single1 ordered1 ordered2` | `single1 = ordered1 >= ordered2` |

### 4.8 文字列連結

| 命令 | 生成されるGoコード |
|---|---|
| `CONCAT single1 slice1 slice2 ...` | `single1 = slice1 + slice2 ...` |

### 4.9 ラベル・分岐

| 命令 | 生成されるGoコード |
|---|---|
| `LABEL label` | `label: ;` |
| `GOTO label` | `goto label` |
| `IF boolean1 label` | `if boolean1 { goto label }` |

### 4.10 関数定義

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `FUNC defname type1 type2 ... : type3 type4 ...` | `func defname(amivm_function_param1 type1, amivm_function_param2 type2 ...) (type3, type4 ...) {` | 関数外 |
| `RET value1 value2 ...` | `return value1, value2 ...` | |
| `ENDFUNC` | `}` | `FUNC`終端 |

### 4.11 関数呼び出し

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `CALL multi1 multi2 ... : callname value1 value2 ...` | `multi1, multi2 ... = callname(value1, value2 ...)` | `multi1`が無い場合は`callname(value1, value2 ...)` |
| `DEFER callname value1 value2 ...` | `defer callname(value1, value2 ...)` | |
| `SPAWN callname value1 value2 ...` | `go callname(value1, value2 ...)` | |

### 4.12 チャネル

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `CHTYPE deftype type1` | `type deftype chan type1` | 関数外 |
| `CHMAKE single1 deftype whole` | `single1 = make(deftype, whole)` | |
| `CHSEND single1 value1` | `single1 <- value1` | |
| `CHRECV multi1 (multi2) single1` | `multi1(, multi2) = <-single1` | |

### 4.13 select

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `SEL` | `select {` | |
| `CASESEND single1 value1 label` | `case single1 <- value1: goto label` | `SEL`内 |
| `CASERECV multi1 (multi2) single1 label` | `case multi1(, multi2) = <-single1: goto label` | `SEL`内 |
| `DEFAULT label` | `default: goto label` | `SEL`内 |
| `ENDSEL` | `}` | `SEL`終端 |

### 4.14 スライス

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `SLTYPE deftype type1` | `type deftype []type1` | 関数外 |
| `SLMAKE single1 deftype whole` | `single1 = make(deftype, whole)` | |
| `SLICE single1 slice1 from to` | `single1 = slice1[from:to]` | |

### 4.15 構造体

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `STTYPE deftype` | `type deftype struct {` | 関数外 |
| `FIELD field type1` | `field type1` | `STTYPE`内 |
| `ENDSTTYPE` | `}` | `STTYPE`終端 |
| `FSET single1 field value1` | `single1.field = value1` | |
| `FGET single1 variable field` | `single1 = variable.field` | |

### 4.16 map

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `MPTYPE deftype type1 type2` | `type deftype map[type1]type2` | 関数外 |
| `MPMAKE single1 deftype` | `single1 = make(deftype)` | |
| `MSET single1 value1 value2` | `single1[value1] = value2` | |
| `MGET multi1 (multi2) single1 value1` | `multi1(, multi2) = single1[value1]` | |
| `MPKEYS single1 single2` | `single1 = slices.Collect(maps.Keys(single2))` | `slices`/`maps`パッケージ(Go標準ライブラリ)を利用。mapを走査する手段として使う |

### 4.17 クロージャー・関数型

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `FNTYPE deftype type1 type2 ... : type3 type4 ...` | `type deftype func(type1, type2 ...) (type3, type4 ...)` | 関数外 |
| `CLOS single1 type1 type2 ... : type3 type4 ...` | `single1 = func(amivm_closureL_param1 type1, amivm_closureL_param2 type2 ...) (type3, type4 ...) {` | `L`はこの`CLOS`のネスト深さ(`FUNC`直下が1、ネストするごとに+1) |
| `ENDCLOS` | `}` | `CLOS`終端 |

## 5. オペランドカテゴリ

| カテゴリ | 説明 | 許容形式 |
|---|---|---|
| `whole` | 0以上の整数(whole number) | `$N` / `&N` / `&L-N` / `%xxx_123` / `@xxx_123` / `@xxx_123.xxx_123` / `0`,`1234` / `'A'` |
| `integer1 integer2` | 整数 | `whole`の形式 + `-1234` |
| `number1 number2` | 数値 | `integer`の形式 + `123.4`,`1.23e4` |
| `boolean1 boolean2` | 真偽値 | `$N` / `&N` / `&L-N` / `%xxx_123` / `@xxx_123` / `@xxx_123.xxx_123` / `true`,`false` |
| `from to` | スライス切り出しの範囲指定 | `$N` / `&N` / `&L-N` / `%xxx_123` / `@xxx_123` / `@xxx_123.xxx_123` / `0`,`1234` / `'A'` / `_`(省略を表す) |
| `slice1 slice2` | スライス・文字列 | `$N` / `&N` / `&L-N` / `%xxx_123` / `@xxx_123` / `@xxx_123.xxx_123` / `"ABC"` |
| `ordered1 ordered2` | 順序比較可能な値 | `$N` / `&N` / `&L-N` / `%xxx_123` / `@xxx_123` / `@xxx_123.xxx_123` / `0`,`1234` / `-1234` / `123.4`,`1.23e4` / `"ABC"` / `'A'` |
| `value1 value2` | 値全般 | `$N` / `&N` / `&L-N` / `%xxx_123` / `@xxx_123` / `@xxx_123.xxx_123` / `true`,`false` / `0`,`1234` / `-1234` / `123.4`,`1.23e4` / `"ABC"` / `'A'` / `nil` / `!xxx_123` / `?xxx_123` / `?xxx_123.xxx_123` |
| `variable` | 変数 | `$N` / `&N` / `&L-N` / `%xxx_123` / `@xxx_123` / `@xxx_123.xxx_123` |
| `local` | `VAR`変数名 | `%xxx_123` |
| `global` | `GVAR`変数名 | `@xxx_123` |
| `single1 single2` | 単一左辺・チャネル変数 | `$N` / `&N` / `&L-N` / `%xxx_123` / `@xxx_123` |
| `multi1 multi2` | 複数左辺 | `$N` / `&N` / `&L-N` / `%xxx_123` / `@xxx_123` / `_` |
| `field` | 構造体フィールド名 | `>xxx_123` |
| `point` | `ADDR`でフィールド/添字を指定する対象 | `$N` / `&N` / `&L-N` / `%xxx_123` / `@xxx_123` / `@xxx_123.xxx_123` / `0`,`1234` / `'A'` / `>xxx_123` |
| `type1 type2 type3 type4` | 型 | `^xxx_123` / `^xxx_123.xxx_123` / `^*xxx_123` / `^*xxx_123.xxx_123` / `^[n]xxx_123` / `^[n]xxx_123.xxx_123` / `^[n]*xxx_123` / `^[n]*xxx_123.xxx_123` |
| `deftype` | 定義型 | `^xxx_123` |
| `defname` | 定義関数名 | `!xxx_123` / `!main` |
| `callname` | 呼び出し関数名 | `!xxx_123` / `!main` / `?xxx_123` / `?xxx_123.xxx_123` / `%xxx_123` / `@xxx_123` / `$N` / `&N` / `&L-N` |
| `label` | ラベル名 | `#xxx_123` |

## 6. トークンの形状分類(Kind)

| トークンの形 | Go生成形 |
|---|---|
| `true` / `false` | そのまま |
| `0` / `1234` | そのまま |
| `-1234` | そのまま |
| `123.4` / `1.23e4` | そのまま |
| `'A'` | そのまま |
| `"ABC"` | そのまま |
| `nil` | そのまま |
| `_` | `from`・`to`の場合は空白、`multiN`の場合は`_` |
| `:` | Goコードには出てこない(関数系命令のデリミタ) |
| `$N` | `amivm_function_paramN` |
| `&N` | `amivm_closureL_paramN`(`L`は自分がいる`CLOS`のネスト深さ) |
| `&L-N` | `amivm_closureL_paramN`(`L`を明示的に指定) |
| `%xxx_123` | `関数名_amivm_function_xxx_123` |
| `@xxx_123` | `xxx_123` |
| `@xxx_123.xxx_123` | `xxx_123.xxx_123` |
| `>xxx_123` | `xxx_123` |
| `^xxx_123` | `xxx_123` |
| `^xxx_123.xxx_123` | `xxx_123.xxx_123` |
| `^*xxx_123` | `*xxx_123` |
| `^*xxx_123.xxx_123` | `*xxx_123.xxx_123` |
| `^[n]xxx_123` | `[n]xxx_123` |
| `^[n]xxx_123.xxx_123` | `[n]xxx_123.xxx_123` |
| `^[n]*xxx_123` | `[n]*xxx_123` |
| `^[n]*xxx_123.xxx_123` | `[n]*xxx_123.xxx_123` |
| `!xxx_123` | `関数名_amivm_function` |
| `!main` | `main` |
| `?xxx_123` | `xxx_123` |
| `?xxx_123.xxx_123` | `xxx_123.xxx_123` |
| `#xxx_123` | `xxx_123` |
