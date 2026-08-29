# AMIVM 仕様書

元は`PJ.txt`(企画メモ)と`IR.txt`(命令セット仕様)という2つの箇条書きファイルだったものを1つに統合し、体系立てて構造化したもの(`PJ.txt`/`IR.txt`は統合後に削除済みで、本ドキュメントがその後継)。その後`METHVAL`/`FUNCVAL`/`INTYPE`/`FUNCM`/`GETYPE`の追加とジェネリクス対応を反映済み。

> **唯一の正確な仕様は本ドキュメント。** 他のドキュメントと矛盾する場合は本ドキュメントを優先する。設計判断の経緯や変更の理由を知りたい場合は`amivm_instruction_spec.md`を参照。

## 目次

1. [概要](#1-概要)
2. [制約・前提条件](#2-制約前提条件)
3. [識別子のプレフィックス](#3-識別子のプレフィックス)
4. [命令一覧](#4-命令一覧)
5. [オペランドカテゴリ](#5-オペランドカテゴリ)
6. [トークンの形状分類](#6-トークンの形状分類)

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

- `FUNC`・`FUNCM`はトップレベルのみに置ける(関数のネスト不可)
- `FUNC`・`FUNCM`・`STTYPE`・`INTYPE`はネスト不可
- `IF`・`LOOP`・`CLOS`・`SEL`はいずれもネストできる。互いの本体の中に、`IF`/`LOOP`/`CLOS`/`SEL`を任意の組み合わせ・任意の深さで書ける(例: `LOOP`の中に`IF`、その中に`CLOS`、その中に`SEL`、というような入れ子も可能)。`CLOS`のネスト深さ(`FUNC`直下を1として数える)は、クロージャー引数`&L-N`の階層番号`L`に対応する
- 配列は1次元固定長のみ
- 多次元配列はAMIVM-IR自体では表現しない。多次元配列はフロントエンド側で1次元に展開する

### 2.2 型定義の前提

- スライス・マップ・構造体・クロージャー(関数型)・チャネルは、対応する`TYPE`系命令(`SLTYPE`/`MPTYPE`/`STTYPE`/`FNTYPE`/`CHTYPE`)で型を定義してから使う

### 2.3 トークン記法

- トークンの区切り文字は**タブ**
- 行頭の空白カラム群(インデント用の連続タブ)は無視する
- カラムの中身はそのまま使う。スペースのトリムなどはしない
- `//`で始まる行はコメント行として無視する
- 命令一覧(4節)の表中に現れる`<<...>>`は表の中だけの省略表記(実際のIR・生成されるGoコードには登場しない)。`...`は可変個の部分を表す

### 2.4 検証方針

- AMIVMは構文レベルの妥当性のみを検証する。生成したGoコードが意味的に正しいか(型の不一致、`break`/`continue`がループの外にある、など)の検証は行わず、Goコンパイラ(`go/types`)に委ねる

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
| `<` | メソッド名 |
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
| `ADDR single1 variable <<point>>` | `point`無し: `single1 = &variable` / `point`が`>xxx_123`: `single1 = &variable.point` / `point`が`>xxx_123`ではない: `single1 = &variable[point]` | |

`ADDR`の例(`point`が無い・フィールド・添字の3パターン):
```
VAR %a ^*Point
ADDR %a @pt
ADDR %a @rect >topLeft
ADDR %a @arr @i
```
↓
```go
var a *Point
a = &pt
a = &rect.topLeft
a = &arr[i]
```

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

### 4.9 ラベル・GOTO

| 命令 | 生成されるGoコード |
|---|---|
| `LABEL label` | `label: ;` |
| `GOTO label` | `goto label` |

`LABEL`/`GOTO`の例:
```
GOTO #done
SET @x 1
LABEL #done
```
↓
```go
goto done
x = 1
done: ;
```

### 4.10 条件分岐(IF)

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `IF boolean1` | `if boolean1 {` | |
| `ELIF boolean1` | `} else if boolean1 {` | |
| `ELSE` | `} else {` | |
| `ENDIF` | `}` | `IF`終端 |

`IF`〜`ENDIF`の間に、`ELIF`(0個以上)・`ELSE`(0個または1個)を書ける。構文規則は以下のとおり。

- `ELIF`は`IF`の直後から`ENDIF`の直前まで、好きな数だけ書ける
- `ELSE`は書くとしても1個のみ
- `ELSE`を書く場合、それは`ELIF`より後・`ENDIF`の直前でなければならない(`ELSE`の後に`ELIF`や別の`ELSE`が続くことはできない)
- `ENDIF`は省略できない

`IF`/`ELIF`/`ELSE`の各本体には、`VAR`宣言を含む任意の命令(`IF`/`LOOP`/`CLOS`/`SEL`のネストを含む)を書ける。

`IF`/`ELIF`(複数)/`ELSE`/`ENDIF`の例:
```
IF @isA
	SET @grade "A"
ELIF @isB
	SET @grade "B"
ELIF @isC
	SET @grade "C"
ELSE
	SET @grade "D"
ENDIF
```
↓
```go
if isA {
	grade = "A"
} else if isB {
	grade = "B"
} else if isC {
	grade = "C"
} else {
	grade = "D"
}
```

### 4.11 ループ(LOOP)

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `LOOP` | `for {` | |
| `BREAK` | `break` | |
| `CONTINUE` | `continue` | |
| `ENDLOOP` | `}` | `LOOP`終端 |

条件付きループ(`while`相当)は`LOOP`の中で`IF`と`BREAK`を組み合わせて表現する(`AMIVM-IR`に条件式付きループ専用の命令は無い)。

`LOOP`/`IF`/`BREAK`を組み合わせた条件付きループの例:
```
LOOP
	IF @done
		BREAK
	ENDIF
	SET @count 1
ENDLOOP
```
↓
```go
for {
	if done {
		break
	}
	count = 1
}
```

`BREAK`/`CONTINUE`は、常に自分を直接囲む最も内側の`LOOP`に対して働く(Goの無名`break`/`continue`と同じ挙動)。ラベル付き`break`/`continue`に相当する機能は無い。

`LOOP`/`IF`/`CONTINUE`の例:
```
LOOP
	IF @skip
		CONTINUE
	ENDIF
	SET @count 1
ENDLOOP
```
↓
```go
for {
	if skip {
		continue
	}
	count = 1
}
```

### 4.12 関数定義

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `FUNC defname <<typename1 constraint1 typename2 constraint2 ... :>> type1 type2 ... : type3 type4 ...` | `func defname <<[typename1 constraint1, typename2 constraint2 ...]>>(amivm_function_param1 type1, amivm_function_param2 type2 ...) (type3, type4 ...) {` | 関数外。レシーバー付きメソッドは`FUNCM`(4.23)を使う |
| `RET value1 value2 ...` | `return value1, value2 ...` | |
| `ENDFUNC` | `}` | `FUNC`終端 |

ジェネリクス無し(引数・戻り値は複数)の`FUNC`の例:
```
FUNC !divmod ^Int ^Int : ^Int ^Int
	VAR %q ^Int
	VAR %r ^Int
	DIV %q $1 $2
	MOD %r $1 $2
	RET %q %r
ENDFUNC
```
↓
```go
func divmod_amivm_function(amivm_function_param1 Int, amivm_function_param2 Int) (Int, Int) {
	var divmod_amivm_function_q Int
	var divmod_amivm_function_r Int
	divmod_amivm_function_q = amivm_function_param1 / amivm_function_param2
	divmod_amivm_function_r = amivm_function_param1 % amivm_function_param2
	return divmod_amivm_function_q, divmod_amivm_function_r
}
```

ジェネリクスあり(型パラメータ・引数・戻り値がそれぞれ複数)の`FUNC`の例:
```
FUNC !pair ^T ^any ^U ^any : ^T ^U : ^T ^U
	RET $1 $2
ENDFUNC
```
↓
```go
func pair_amivm_function[T any, U any](amivm_function_param1 T, amivm_function_param2 U) (T, U) {
	return amivm_function_param1, amivm_function_param2
}
```

### 4.13 関数呼び出し

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `CALL multi1 multi2 ... : callname <<type1 type2 ... :>> value1 value2 ...` | `multi1, multi2 ... = callname<<[type1, type2 ...]>>(value1, value2 ...)` | `multi1`が無い場合は`callname<<[...]>>(value1, value2 ...)` |
| `DEFER callname <<type1 type2 ... :>> value1 value2 ...` | `defer callname<<[type1, type2 ...]>>(value1, value2 ...)` | |
| `SPAWN callname <<type1 type2 ... :>> value1 value2 ...` | `go callname<<[type1, type2 ...]>>(value1, value2 ...)` | |

`CALL`で複数の戻り値(`multi1 multi2 ...`)を受け取る例:
```
CALL @result @err : !fetchData "https://example.com"
```
↓
```go
result, err = fetchData_amivm_function("https://example.com")
```

`CALL`に明示的な型引数(`type1 type2 ...`)を複数付けて呼ぶ例:
```
CALL @p1 @p2 : !pair ^Int ^String : @x @y
```
↓
```go
p1, p2 = pair_amivm_function[Int, String](x, y)
```

`CALL`で関数でないもの(Goの型変換)を呼ぶ例:
```
CALL @s : ?string @n
```
↓
```go
s = string(n)
```

`DEFER`でレシーバー付きメソッドを引数無しで呼ぶ例:
```
DEFER ?file.Close
```
↓
```go
defer file.Close()
```

`DEFER`で複数の引数(`value1 value2 ...`)を渡す例:
```
DEFER !logAccess @user @path
```
↓
```go
defer logAccess_amivm_function(user, path)
```

`SPAWN`で複数の引数(`value1 value2 ...`)を渡す例:
```
SPAWN !sendLog "started" @requestId
```
↓
```go
go sendLog_amivm_function("started", requestId)
```

### 4.14 チャネル

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `CHTYPE typename1 type1` | `type typename1 chan type1` | 関数外 |
| `CHMAKE single1 typename1 whole` | `single1 = make(typename1, whole)` | |
| `CHSEND single1 value1` | `single1 <- value1` | |
| `CHRECV multi1 <<multi2>> single1` | `multi1<<, multi2>> = <-single1` | |

### 4.15 select

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `SEL` | `select {` | |
| `CASESEND single1 value1` | `case single1 <- value1:` | `SEL`内。以降`ENDSEL`か次の`CASESEND`/`CASERECV`/`DEFAULT`までが本体 |
| `CASERECV multi1 <<multi2>> single1` | `case multi1<<, multi2>> = <-single1:` | `SEL`内。同上 |
| `DEFAULT` | `default:` | `SEL`内。同上 |
| `ENDSEL` | `}` | `SEL`終端 |

次の`CASESEND`/`CASERECV`/`DEFAULT`または`ENDSEL`が現れるまでの範囲が、その節の本体になる。本体には`VAR`宣言を含む任意の命令(`IF`/`LOOP`/`CLOS`/`SEL`のネストを含む)を書ける。`BREAK`は`LOOP`が無ければ`SEL`自体を抜ける。

`SEL`/`CASERECV`(複数)/`DEFAULT`/`ENDSEL`の例:
```
SEL
	CASERECV @v1 @ch1
		SET @result @v1
	CASERECV @v2 @ch2
		SET @result @v2
	DEFAULT
		SET @result 0
ENDSEL
```
↓
```go
select {
case v1 = <-ch1:
	result = v1
case v2 = <-ch2:
	result = v2
default:
	result = 0
}
```

`LOOP`が無い場合に`BREAK`が`SEL`自体を抜ける例:
```
SEL
	CASERECV @v @ch
		BREAK
	DEFAULT
		SET @result 0
ENDSEL
```
↓
```go
select {
case v = <-ch:
	break
default:
	result = 0
}
```

### 4.16 配列

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `ARTYPE typename1 type1 imm` | `type typename1 [imm]type1` | 関数外。配列は本命令を使わず`^[n]type1`という複合形でインラインに宣言することもできる(2.1節・6節参照)。`ARTYPE`は名前を付けて再利用したい場合の選択肢であり、必須ではない |

`ARTYPE`の例:
```
ARTYPE ^Board ^Int 9
```
↓
```go
type Board [9]Int
```

### 4.17 スライス

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `SLTYPE typename1 type1` | `type typename1 []type1` | 関数外 |
| `SLMAKE single1 typename1 whole` | `single1 = make(typename1, whole)` | |
| `SLICE single1 slice1 from to` | `single1 = slice1[from:to]` | |

`SLTYPE`の例:
```
SLTYPE ^IntList ^Int
```
↓
```go
type IntList []Int
```

`SLMAKE`/`SLICE`(`from`/`to`省略パターンを含む)の例:
```
SLMAKE @list ^IntList 0
SLICE @part @list 0 3
SLICE @head @list _ 3
SLICE @tail @list 3 _
```
↓
```go
list = make(IntList, 0)
part = list[0:3]
head = list[:3]
tail = list[3:]
```

### 4.18 構造体

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `STTYPE typename1 <<typename2 constraint1 typename3 constraint2 ...>>` | `type typename1 <<[typename2 constraint1, typename3 constraint2 ...]>> struct {` | 関数外 |
| `FIELD field type1` | `field type1` | `STTYPE`内 |
| `ENDSTTYPE` | `}` | `STTYPE`終端 |
| `FSET single1 field value1` | `single1.field = value1` | |
| `FGET single1 variable field` | `single1 = variable.field` | |

`STTYPE`/`FIELD`(複数)/`ENDSTTYPE`の例:
```
STTYPE ^Point
	FIELD >x ^Float
	FIELD >y ^Float
ENDSTTYPE
```
↓
```go
type Point struct {
	x Float
	y Float
}
```

`FSET`/`FGET`の例:
```
FSET @p >x 0
FSET @p >y 0
FGET @x2 @p >x
```
↓
```go
p.x = 0
p.y = 0
x2 = p.x
```

型パラメータを1つ持つ構造体の例(`FUNCM`の`Box`の宣言側。使用例は4.23節を参照):
```
STTYPE ^Box ^T ^any
	FIELD >value ^T
ENDSTTYPE
```
↓
```go
type Box[T any] struct {
	value T
}
```

型パラメータを複数持つ構造体の例:
```
STTYPE ^Pair ^T ^any ^U ^any
	FIELD >first ^T
	FIELD >second ^U
ENDSTTYPE
```
↓
```go
type Pair[T any, U any] struct {
	first T
	second U
}
```

### 4.19 map

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `MPTYPE typename1 type1 type2` | `type typename1 map[type1]type2` | 関数外 |
| `MPMAKE single1 typename1` | `single1 = make(typename1)` | |
| `MSET single1 value1 value2` | `single1[value1] = value2` | |
| `MGET multi1 <<multi2>> single1 value1` | `multi1<<, multi2>> = single1[value1]` | |
| `MPKEYS single1 single2` | `single1 = slices.Collect(maps.Keys(single2))` | `slices`/`maps`パッケージ(Go標準ライブラリ)を利用。mapを走査する手段として使う |

`MPTYPE`の例:
```
MPTYPE ^Scores ^String ^Int
```
↓
```go
type Scores map[String]Int
```

`MSET`/`MGET`(`multi2`で`ok`を受け取る形)/`MPKEYS`の例:
```
MSET @scores "alice" 100
MGET @score @ok @scores "alice"
MPKEYS @names @scores
```
↓
```go
scores["alice"] = 100
score, ok = scores["alice"]
names = slices.Collect(maps.Keys(scores))
```

### 4.20 クロージャー・関数型

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `FNTYPE typename1 type1 type2 ... : type3 type4 ...` | `type typename1 func(type1, type2 ...) (type3, type4 ...)` | 関数外 |
| `CLOS single1 type1 type2 ... : type3 type4 ...` | `single1 = func(amivm_closureL_param1 type1, amivm_closureL_param2 type2 ...) (type3, type4 ...) {` | `L`はこの`CLOS`のネスト深さ(`FUNC`直下が1、ネストするごとに+1) |
| `ENDCLOS` | `}` | `CLOS`終端 |

引数・戻り値がそれぞれ複数ある`FNTYPE`の例:
```
FNTYPE ^Adder ^Int ^Int : ^Int ^Boolean
```
↓
```go
type Adder func(Int, Int) (Int, Boolean)
```

`CLOS`/`ENDCLOS`(クロージャー引数`&N`を複数使う)の例:
```
VAR %adder ^Adder
CLOS %adder ^Int ^Int : ^Int ^Boolean
	VAR %sum ^Int
	ADD %sum &1 &2
	RET %sum true
ENDCLOS
```
↓
```go
var adder Adder
adder = func(amivm_closure1_param1 Int, amivm_closure1_param2 Int) (Int, Boolean) {
	var sum Int
	sum = amivm_closure1_param1 + amivm_closure1_param2
	return sum, true
}
```

### 4.21 型アサーション

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `ASSERT multi1 <<multi2>> variable type1` | `multi1<<, multi2>> = variable.(type1)` | `multi2`省略時は1つの代入(失敗時にpanicする単一形)。指定時は2つ(値, ok)の代入になり、失敗してもpanicしない |

`ASSERT`で失敗を許容せずpanicさせる形(`multi2`省略)の例:
```
ASSERT @n @x ^Int
```
↓
```go
n = x.(Int)
```

`ASSERT`で失敗を許容する形(`multi2`あり、`ok`を受け取る)の例:
```
ASSERT @n @ok @x ^Int
```
↓
```go
n, ok = x.(Int)
```

### 4.22 メソッド値・関数値の取得

無名スライス型・無名map型・無名構造体型などを含むメソッド・関数を値として取り出す時に使う(`FNTYPE`+`FGET`では型が一致せず取得できない場合がある)。

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `METHVAL local variable method` | `local := variable.method` | レシーバー付きメソッド専用。`local`は`VAR`で事前宣言しない(`%xxx_123`のみ)。`:=`で型推論しながら新規宣言する |
| `FUNCVAL local callname` | `local := callname` | レシーバーを持たない関数値(Goライブラリのパッケージ関数・AMIVM関数・既存の関数値変数)を値として取得する。`callname`は`CALL`と同じオペランドカテゴリ(`!xxx_123` / `?xxx_123` / `?xxx_123.xxx_123` / `%xxx_123` / `@xxx_123` / `$N` / `&N` / `&L-N`)を使う。`local`は`VAR`で事前宣言しない(`%xxx_123`のみ)。`:=`で型推論しながら新規宣言する |

`METHVAL`の例(先に変数へ代入した`file`のメソッドを値として取得):
```
CALL @file @err : ?os.Open "data.txt"
METHVAL %closeFn @file <Close
```
↓
```go
file, err = os.Open("data.txt")
closeFn := file.Close
```

`FUNCVAL`の例:
```
FUNCVAL %f ?strings.ToUpper
```
↓
```go
f := strings.ToUpper
```

### 4.23 メソッド定義

`STTYPE`の構造体にGoのネイティブなレシーバー付きメソッドを持たせる時に使う(`FUNC`はレシーバーを持たない普通の関数のみ)。

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `FUNCM defname receiver <<typename1 typename2 ... :>> type1 type2 ... : type3 type4 ...` | `func (amivm_method_self receiver<<[typename1, typename2 ...]>>) defname(amivm_function_param1 type1, amivm_function_param2 type2 ...) (type3, type4 ...) {` | 関数外。`receiver`がレシーバー型、`type1`以降が引数型、`:`の後(`type3`以降)が戻り値型。`<<typename1 typename2 ... :>>`は`STTYPE`側で宣言済みの型パラメータ名の再掲であり、新規宣言ではない(制約は書けない)。Goの言語仕様上、メソッドは独自の型パラメータを宣言できないため |
| `RET value1 value2 ...` | `return value1, value2 ...` | `FUNC`と共用 |
| `ENDFUNCM` | `}` | `FUNCM`終端 |

`FUNCM`の本体内では、レシーバーは`$0`として参照する(`$1`以降は通常の引数と同じ)。

ジェネリクス無し(引数・戻り値は複数)の`FUNCM`(レシーバー`$0`を使う)の例:
```
FUNCM !scale ^*Circle ^Float ^Float : ^Float ^Float
	VAR %area ^Float
	MUL %area $0 $0
	VAR %scaledArea ^Float
	MUL %scaledArea %area $1
	VAR %scaledRadius ^Float
	MUL %scaledRadius $0 $2
	RET %scaledArea %scaledRadius
ENDFUNCM
```
↓
```go
func (amivm_method_self *Circle) scale_amivm_function(amivm_function_param1 Float, amivm_function_param2 Float) (Float, Float) {
	var scale_amivm_function_area Float
	scale_amivm_function_area = amivm_method_self * amivm_method_self
	var scale_amivm_function_scaledArea Float
	scale_amivm_function_scaledArea = scale_amivm_function_area * amivm_function_param1
	var scale_amivm_function_scaledRadius Float
	scale_amivm_function_scaledRadius = amivm_method_self * amivm_function_param2
	return scale_amivm_function_scaledArea, scale_amivm_function_scaledRadius
}
```

ジェネリクスあり・型パラメータ1つの`FUNCM`の例(`^Box`は4.18節で宣言したもの):
```
FUNCM !get ^*Box ^T : : ^T
	RET $0
ENDFUNCM
```
↓
```go
func (amivm_method_self *Box[T]) get_amivm_function() (T) {
	return amivm_method_self
}
```

ジェネリクスあり・型パラメータが複数の`FUNCM`の例(`^Pair`は4.18節で宣言したもの):
```
FUNCM !swap ^*Pair ^T ^U : : ^U ^T
	VAR %a ^U
	VAR %b ^T
	FGET %a $0 >second
	FGET %b $0 >first
	RET %a %b
ENDFUNCM
```
↓
```go
func (amivm_method_self *Pair[T, U]) swap_amivm_function() (U, T) {
	var swap_amivm_function_a U
	var swap_amivm_function_b T
	swap_amivm_function_a = amivm_method_self.second
	swap_amivm_function_b = amivm_method_self.first
	return swap_amivm_function_a, swap_amivm_function_b
}
```

### 4.24 インターフェース型定義

インターフェースはメソッドシグネチャの羅列を持つ(`STTYPE`のフィールドの羅列に相当)。

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `INTYPE typename1 <<typename2 constraint1 typename3 constraint2 ...>>` | `type typename1 <<[typename2 constraint1, typename3 constraint2 ...]>> interface {` | 関数外 |
| `METHOD method type1 type2 ... : type3 type4 ...` | `method(type1, type2 ...) (type3, type4 ...)` | レシーバーを持たない(シグネチャのみ)。`STTYPE`の`FIELD`に相当。`INTYPE`側で導入した型パラメータ名は通常の`type`カテゴリとしてそのまま参照できるので、`METHOD`自体は独自の`<<>>`を持たない(`FUNCM`と同じ理由)。インターフェース内のメソッドシグネチャなので生成コードに`func`キーワードは付かない。`type1 type2 ...`がいつ終わるか構文上判別できないため、戻り値が0個でも`:`は省略できない(`FUNC`/`CALL`/`DEFER`/`SPAWN`/`FUNCM`と同じ厳格なルール) |
| `ENDINTYPE` | `}` | `INTYPE`終端 |

`INTYPE`/`METHOD`/`ENDINTYPE`の例:
```
INTYPE ^Ord
	METHOD <compareTo ^Ord : ^Int
ENDINTYPE
```
↓
```go
type Ord interface {
	compareTo(Ord) (Int)
}
```

型パラメータを複数持つジェネリクスなインターフェースの例:
```
INTYPE ^KVStore ^K ^comparable ^V ^any
	METHOD <get ^K : ^V
	METHOD <set ^K ^V :
ENDINTYPE
```
↓
```go
type KVStore[K comparable, V any] interface {
	get(K) (V)
	set(K, V)
}
```

### 4.25 ジェネリクス型の実体化(別名宣言)

`STTYPE`/`INTYPE`等でジェネリクスに宣言した型(`Box<T>`等)は、そのままでは`type`カテゴリ(5節)のトークンとして参照できない(型引数を当てはめる場所が無い)。型引数を当てはめて確定させた具体型に、別名を宣言することで、以降は普通の`type`カテゴリのトークンとしてどこでも使えるようにする。

| 命令 | 生成されるGoコード | 備考 |
|---|---|---|
| `GETYPE typename1 typename2 type1 type2 ...` | `type typename1 = typename2[type1, type2, ...]` | 関数外。`typename1`が新しく宣言する別名、`typename2`が実体化する対象のジェネリクス型名、`type1`以降が当てはめる型引数。`=`によるエイリアス宣言なので、`typename1`と`typename2[type1, ...]`は同じ型として扱われる(相互変換不要) |

型引数が1つの`GETYPE`の例:
```
GETYPE ^BoxInt ^Box ^Int
```
↓
```go
type BoxInt = Box[Int]
```

型引数(`type1 type2 ...`)が複数の`GETYPE`の例:
```
GETYPE ^PairIntString ^Pair ^Int ^String
```
↓
```go
type PairIntString = Pair[Int, String]
```

これで`^BoxInt`は普通の`type`カテゴリのトークンとして、型を書けるあらゆる場所で使える(`VAR`/`FGET`の例):
```
VAR %b ^BoxInt
FGET %v %b >value
```
↓
```go
var b BoxInt
v = b.value
```

## 5. オペランドカテゴリ

| カテゴリ | 説明 | 許容形式 |
|---|---|---|
| `imm` | 即値(コンパイル時定数のリテラル。識別子は不可) | `0`,`1234` / `'A'` |
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
| `method` | メソッド名 | `<xxx_123` |
| `point` | `ADDR`でフィールド/添字を指定する対象 | `$N` / `&N` / `&L-N` / `%xxx_123` / `@xxx_123` / `@xxx_123.xxx_123` / `0`,`1234` / `'A'` / `>xxx_123` |
| `type1 type2 type3 type4` | 型 | `^xxx_123` / `^xxx_123.xxx_123` / `^*xxx_123` / `^*xxx_123.xxx_123` / `^[n]xxx_123` / `^[n]xxx_123.xxx_123` / `^[n]*xxx_123` / `^[n]*xxx_123.xxx_123` |
| `receiver` | `FUNCM`のレシーバー型 | `^xxx_123` / `^*xxx_123` |
| `typename1 typename2 typename3` | 定義型・型パラメータ名 | `^xxx_123` |
| `constraint1 constraint2` | 型パラメータの制約 | `^xxx_123` / `^xxx_123.xxx_123` |
| `defname` | 定義関数名 | `!xxx_123` / `!main` |
| `callname` | 呼び出し関数名 | `!xxx_123` / `!main` / `?xxx_123` / `?xxx_123.xxx_123` / `%xxx_123` / `@xxx_123` / `$N` / `&N` / `&L-N` |
| `label` | ラベル名 | `#xxx_123` |

`$N`の`N`は通常`1`以上(`FUNC`/`CLOS`の引数)。**`FUNCM`の本体内に限り`$0`(レシーバー)も使える**。`$0`は`$N`が許容される全カテゴリで使えるが、例外として`single`・`multi`(代入先・左辺値)では使えない——レシーバー自体への再代入は認めない設計とする。

上表で`0`,`1234`のように10進の例だけを挙げている箇所(`imm`/`whole`/`from`・`to`/`integer`/`ordered`/`value`/`point`)は、いずれも6節で定義する整数リテラルの全形式(10進・16進・8進・2進、桁区切りの`_`を含む)を等しく許容する。表中では代表例として10進のみを記載している。

## 6. トークンの形状分類

| トークンの形 | Go生成形 |
|---|---|
| `true` / `false` | そのまま |
| `0` / `1234` / `0x1A` / `0o17` / `0b101` / `1_000_000` | そのまま。整数リテラルは10進(`0`単体、または`1`-`9`に`0`-`9`が続く形)・16進(`0x`/`0X`に`0`-`9`・`a`-`f`・`A`-`F`が続く形)・8進(`0o`/`0O`に`0`-`7`が続く形)・2進(`0b`/`0B`に`0`-`1`が続く形)のいずれかで書ける。桁区切りの`_`は、基数プレフィックス(`0x`等)の直後、または数字と数字の間にのみ置ける(先頭・末尾・`_`の連続、およびプレフィックス無しの`0`単体への付加は不可) |
| `-1234` / `-0x1A` / `-0o17` / `-0b101` | そのまま。上記いずれの整数リテラルにも`-`を前置できる(符号付き整数) |
| `123.4` / `1.23e4` | そのまま |
| `'A'` | そのまま。`'\n'`等の名前付きエスケープ(`\a`/`\b`/`\f`/`\n`/`\r`/`\t`/`\v`/`\\`/`\'`/`\"`)、`\uXXXX`(4桁hex)・`\UXXXXXXXX`(8桁hex)によるUnicodeコードポイント指定、非ASCII文字(`'あ'`等)も許容する。`\xHH`(2桁hexバイト値)・8進数バイト値エスケープ(`\nnn`)は非対応(`\u`/`\U`でコードポイントを直接指定できるため) |
| `"ABC"` | そのまま。`\"`を含むエスケープシーケンスを許容する(個々のエスケープの妥当性はGo側のパース・型チェックに委ねる) |
| `nil` | そのまま |
| `_` | `from`・`to`の場合は空白、`multiN`の場合は`_` |
| `:` | Goコードには出てこない(関数系命令のデリミタ) |
| `$0` | `amivm_method_self`(`FUNCM`本体内のみ) |
| `$N`(`N`≥`1`) | `amivm_function_paramN` |
| `&N` | `amivm_closureL_paramN`(`L`は自分がいる`CLOS`のネスト深さ) |
| `&L-N` | `amivm_closureL_paramN`(`L`を明示的に指定) |
| `%xxx_123` | `関数名_amivm_function_xxx_123` |
| `@xxx_123` | `xxx_123` |
| `@xxx_123.xxx_123` | `xxx_123.xxx_123` |
| `>xxx_123` | `xxx_123` |
| `<xxx_123` | `xxx_123` |
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
