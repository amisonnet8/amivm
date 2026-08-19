# AMIVM IR命令セット仕様

本ドキュメントは`main.go`の実装に対応する解説版であり、**唯一の正確な仕様は`amivm_spec.md`**(プロジェクトルート)である。本ドキュメントと`amivm_spec.md`が矛盾する場合は`amivm_spec.md`を優先すること。

## 0. トークンの区切り文字・コメント

各命令のトークンは**タブ文字**で区切る。文字列リテラル(`"hello world"`のようにスペースを含むもの)が誤って複数トークンに分割されるのを避けるため。行頭の空白カラム群(インデント用の連続タブ)は無視し、それ以外のカラムはトリムせずそのまま使う。

`//`で始まる行はコメント行として無視する(行全体がコメントの場合のみ対応。行末コメントは非対応)。

## 1. 識別子のプレフィックス体系

全ての識別子(変数・型・関数・ラベル・構造体フィールド)は、種別を表す記号を先頭に持つ。プレフィックスなしの裸の識別子が出現するのは、型宣言(`^`)や構造体フィールド宣言(`FIELD`)の中の要素名部分のみ。

| プレフィックス | 意味 | Go生成名 |
|---|---|---|
| `$` | 関数引数(パラメータ) | `$N` → `<関数名>_amivm_function_paramN` |
| `&` | クロージャー引数 | `&N` → `amivm_closure_paramN`(関数名による修飾なし) |
| `%` | 関数内変数名 | `%xxx` → `<関数名>_amivm_function_xxx` |
| `@` | 関数外(パッケージレベル)変数名 | `@xxx` はそのまま、`@pkg.xxx` は他パッケージの値参照 |
| `^` | 型名 | `^xxx` → `xxx`(7節参照) |
| `>` | 構造体フィールド名 | `>xxx` → `xxx` |
| `!` | AMIVM内定義関数名 | `!xxx` → `xxx_amivm_function`、`!main` → `main`のまま |
| `?` | Go関数名 | そのまま使う(`?xxx.yyy`はパッケージセレクタ) |
| `#` | ラベル名 | そのまま使う(Goのラベル構文にプレフィックスは含めない) |

宣言側にも参照側と同じプレフィックスを要求する(`VAR`は`%`のみ、`GVAR`は`@`のみ)。これにより「関数内で`@`を使って宣言する」「トップレベルで`%`を使う」といった文脈のミスマッチを、宣言の時点でパースエラーとして検出できる。

識別子(`$`/`&`/`%`/`@`)には添字・スライス・デリファレンス・アドレス取得を埋め込んだ複合形は存在しない。それらは全て専用命令(`ASET`/`AGET`/`PSET`/`PGET`/`ADDR`/`SLICE`)に分離されている。そのため識別子側のKindは`$N`/`&N`/`%xxx`/`@xxx`/`@xxx.yyy`の5種類のみで、`@xxx.yyy`(他パッケージ参照)以外は全カテゴリで共通に扱える。

## 2. トップレベルの制約

関数の外(トップレベル)に置けるのは`GVAR`・`FUNC`・`CHTYPE`・`SLTYPE`・`STTYPE`・`MPTYPE`・`FNTYPE`のみ。`GVAR`はGoのパッケージレベル変数になる。`FUNC`のみ関数本体を持つブロックとして関数外に置け、それ以外の命令(演算・制御フロー等)は`FUNC`〜`ENDFUNC`の中でのみ使用できる。`FUNC`・`STTYPE`・`CLOS`・`SEL`はいずれもネスト不可。

## 3. 命令一覧

| IR命令(タブ区切り) | Go生成コード |
|---|---|
| `VAR local type1` | `var local type1`(関数内のみ) |
| `GVAR global type1` | `var global type1`(関数外のみ) |
| `SET single value1` | `single = value1` |
| `ASET single whole value1` | `single[whole] = value1` |
| `AGET single variable whole` | `single = variable[whole]` |
| `PSET single value1` | `*single = value1` |
| `PGET single variable` | `single = *variable` |
| `ADDR single variable` | `single = &variable` |
| `ADD single number1 number2` | `single = number1 + number2` |
| `SUB single number1 number2` | `single = number1 - number2` |
| `MUL single number1 number2` | `single = number1 * number2` |
| `DIV single number1 number2` | `single = number1 / number2` |
| `MOD single integer1 integer2` | `single = integer1 % integer2` |
| `BAND single integer1 integer2` | `single = integer1 & integer2` |
| `BOR single integer1 integer2` | `single = integer1 \| integer2` |
| `BXOR single integer1 integer2` | `single = integer1 ^ integer2` |
| `BCLEAR single integer1 integer2` | `single = integer1 &^ integer2` |
| `BNOT single integer1` | `single = ^integer1` |
| `SHL single integer1 whole` | `single = integer1 << whole` |
| `SHR single integer1 whole` | `single = integer1 >> whole` |
| `AND single boolean1 boolean2` | `single = boolean1 && boolean2` |
| `OR single boolean1 boolean2` | `single = boolean1 \|\| boolean2` |
| `NOT single boolean1` | `single = !boolean1` |
| `EQ single value1 value2` | `single = value1 == value2` |
| `NEQ single value1 value2` | `single = value1 != value2` |
| `LT single ordered1 ordered2` | `single = ordered1 < ordered2` |
| `LTE single ordered1 ordered2` | `single = ordered1 <= ordered2` |
| `GT single ordered1 ordered2` | `single = ordered1 > ordered2` |
| `GTE single ordered1 ordered2` | `single = ordered1 >= ordered2` |
| `CONCAT single slice1 slice2 ...` | `single = slice1 + slice2 ...` |
| `LABEL label` | `label: ;` |
| `GOTO label` | `goto label` |
| `IF boolean1 label` | `if boolean1 { goto label }` |
| `FUNC defname type1 type2 ... : type3 type4 ...` | `func defname(param1 type1, param2 type2 ...) (type3, type4 ...) {` |
| `RET value1 value2 ...` | `return value1, value2 ...` |
| `ENDFUNC` | `}`(`FUNC`終端) |
| `CALL multi1 multi2 ... : callname value1 value2 ...` | `multi1, multi2 ... = callname(value1, value2 ...)`(`multi1`が無ければ式文として呼ぶだけ) |
| `DEFER callname value1 value2 ...` | `defer callname(value1, value2 ...)` |
| `SPAWN callname value1 value2 ...` | `go callname(value1, value2 ...)` |
| `CHTYPE deftype type1` | `type deftype chan type1`(関数外) |
| `CHMAKE single deftype whole` | `single = make(deftype, whole)` |
| `CHSEND single value1` | `single <- value1` |
| `CHRECV multi1 (multi2) single` | `multi1(, multi2) = <-single` |
| `SEL` | `select {` |
| `CASESEND single value1 label` | `case single <- value1: goto label`(`SEL`内) |
| `CASERECV multi1 (multi2) single label` | `case multi1(, multi2) = <-single: goto label`(`SEL`内) |
| `DEFAULT label` | `default: goto label`(`SEL`内) |
| `ENDSEL` | `}`(`SEL`終端) |
| `SLTYPE deftype type1` | `type deftype []type1`(関数外) |
| `SLMAKE single deftype whole` | `single = make(deftype, whole)` |
| `SLICE single slice1 from to` | `single = slice1[from:to]` |
| `STTYPE deftype` | `type deftype struct {`(関数外) |
| `FIELD field type1` | `field type1`(`STTYPE`内) |
| `ENDSTTYPE` | `}`(`STTYPE`終端) |
| `FSET single field value1` | `single.field = value1` |
| `FGET single variable field` | `single = variable.field` |
| `MPTYPE deftype type1 type2` | `type deftype map[type1]type2`(関数外) |
| `MPMAKE single deftype` | `single = make(deftype)` |
| `MSET single value1 value2` | `single[value1] = value2` |
| `MGET multi1 (multi2) single value1` | `multi1(, multi2) = single[value1]` |
| `FNTYPE deftype type1 type2 ... : type3 type4 ...` | `type deftype func(type1, type2 ...) (type3, type4 ...)`(関数外) |
| `CLOS local type1 type2 ... : type3 type4 ...` | `local = func(&1 type1, &2 type2 ...) (type3, type4 ...) {` |
| `ENDCLOS` | `}`(`CLOS`終端) |

`ADD`は数値演算専用(常に2オペランド)。文字列連結は`CONCAT`(可変長引数)で行う。キャスト(型変換)・組み込み関数(`close`/`len`/`cap`等)は専用命令を持たず、`CALL`に統合している(9節参照)。

### `:`(コロン)区切り

`CALL`・`FUNC`・`FNTYPE`・`CLOS`の4命令は、トークン列の中に必ず1個の`:`を含み、それを境に「代入先・関数名」の側と「呼び出し対象・パラメータ列/戻り値型」の側を分割する。

- `FUNC`/`FNTYPE`: `:`より前がパラメータ型列、後が戻り値型列
- `CALL`: `:`より前が代入先(`multi`列、0個以上)、後が呼び出し対象+引数列
- `CLOS`: `local`の次から`:`より前がパラメータ型列、後が戻り値型列

`:`が無い、または2個以上あるとパースエラーになる(パラメータ・戻り値・代入先が0個の場合でも`:`自体は省略できない)。`DEFER`/`SPAWN`は`callname value1 value2 ...`のみで`:`を使わない。

### `SEL`/`CASESEND`/`CASERECV`/`DEFAULT`/`ENDSEL`

`CASESEND`/`CASERECV`は(旧`CASE SEND`/`CASE RECV`から)1語化されている。`CHRECV`/`CASERECV`の`(multi2)`は省略可能。省略時は1つ、指定時は2つ(値, ok)の代入になる。`SEL`は`FUNC`内にのみ出現する。`CASESEND`/`CASERECV`/`DEFAULT`の中身は1行で完結し、指定したラベルへ`goto`する。

### 変数は常に「宣言 → `=`で代入」に統一する

チャネル・スライス・マップ・構造体変数も含め、全ての変数は`VAR`/`GVAR`で事前に宣言してから代入する形に統一する。`CHMAKE`/`SLMAKE`/`MPMAKE`は新規宣言(`:=`)ではなく、既存変数への代入(`=`)として扱う。同様に`CASERECV`のような受信処理も含め、あらゆる代入命令が一貫して`=`(既存変数への代入)を使う設計になっており、`:=`によるスコープ内シャドーイングが構造的に発生しない(過去にこの点で実際にバグを踏んだため、設計として固定している)。

## 4. VAR/GVAR/CLOSの命名規則(未使用変数対応との関連)

関数内変数(`VAR`, `%`)は、Go側の実際の変数名を`<関数名>_amivm_function_<変数名>`という形にする。パラメータ(`$N`)も同様に`<関数名>_amivm_function_paramN`とする。`main`関数も例外なくこの規則に従う(関数宣言自体の名前は`main`のまま、変数名prefixとしては`main_amivm_function_`を使う)。

クロージャー引数(`&N`)は、関数名による修飾を持たず常に`amivm_closure_paramN`という固定名になる。`CLOS`は無名の`func`リテラルとして展開され、Goの`func`リテラルはそれぞれ独立したブロックスコープを持つため、複数の`CLOS`が同じ`amivm_closure_paramN`を使っても衝突しない。`CLOS`本体内で`VAR`宣言した変数(`%xxx`)は、`CLOS`を包む外側の`FUNC`の名前で修飾される(`CLOS`自体は無名なので専用の名前空間を持たない)。

パッケージレベル変数(`GVAR`, `@`)は、命名規則を変更せずそのままの変数名を使う。Goは未使用のパッケージレベル変数をコンパイルエラーにしないため、命名規則による対応が不要なことに対応している。

この命名規則により、`go/types`が返す`declared and not used`エラーメッセージの変数名から、`_amivm_function_`という固定マーカー文字列で分割するだけで所属関数を特定できる。

未使用変数の`_ = x`挿入位置は、該当する`VAR`宣言文の直後にする。関数末尾に追加すると、戻り値のある関数で最後の文が`return`でなくなり`missing return`エラーを誘発するため。挿入先の探索は関数のトップレベル文だけでなく、`IF`本体・`SEL`の各`CommClause`・`CLOS`(`func`リテラル)の内部まで再帰的に辿る(`CLOS`はネストしたブロックスコープを作るため、これを省略すると`CLOS`内で未使用になった変数を救済できない)。

## 5. オペランドカテゴリ

各カテゴリの識別子側の許容範囲は、`$N`/`&N`/`%xxx`/`@xxx`に加えて`@xxx.yyy`(他パッケージ参照)を含むかどうかで大きく2系統に分かれる。

- **フル系**(`@xxx.yyy`含む): `whole`/`integer`/`number`/`boolean`/`from`・`to`/`slice`/`ordered`/`value`/`variable`
- **単純系**(`@xxx.yyy`含まない): `single`/`multi`(`multi`はさらに`_`を許容)

| カテゴリ | 意味 | 許容形式 |
|---|---|---|
| `local` | `VAR`宣言名(関数内) | `%xxx` |
| `global` | `GVAR`宣言名(関数外) | `@xxx` |
| `single` | 単一左辺・チャネル/スライス/マップ/構造体変数の参照 | `$N` / `&N` / `%xxx` / `@xxx` |
| `multi1 multi2` | 複数左辺(`CALL`/`CHRECV`/`CASERECV`/`MGET`の代入先) | `single`の形式 + `_` |
| `variable` | 読み取り専用の変数参照(`AGET`/`PGET`/`ADDR`/`FGET`の読み出し元) | `$N` / `&N` / `%xxx` / `@xxx` / `@xxx.yyy` |
| `field` | 構造体フィールド名 | `>xxx` |
| `type1 type2 type3 type4` | 型表現(7節) | `^xxx`系8パターン |
| `deftype` | `TYPE`系命令で宣言・参照する定義型名 | `^xxx`(単純形のみ) |
| `whole` | 0以上の整数(添字・シフト量・サイズ等) | フル系識別子 + `0`,`1234` / `'A'` |
| `from to` | `SLICE`の範囲指定 | `whole`の形式 + `_`(省略。Go側では空欄になる) |
| `integer1 integer2` | 整数オペランド | `whole`の形式 + `-1234` |
| `number1 number2` | 数値オペランド | `integer`の形式 + `123.4`,`1.23e4` |
| `boolean1 boolean2` | 真偽値オペランド | フル系識別子 + `true`,`false` |
| `slice1 slice2` | スライス・文字列(`CONCAT`/`SLICE`の対象) | フル系識別子 + `"ABC"` |
| `ordered1 ordered2` | 順序比較可能な値 | フル系識別子 + `0`,`1234`,`-1234` / `123.4`,`1.23e4` / `"ABC"` / `'A'` |
| `value1 value2` | 値全般(最も緩い) | `ordered`の形式 + `true`,`false` / `nil` |
| `defname` | AMIVM内で定義する関数名 | `!xxx` / `!main` |
| `callname` | 呼び出し対象 | `!xxx` / `!main` / `?xxx` / `?xxx.yyy` / `%xxx`(関数値・メソッド値・クロージャーを保持するローカル変数) |
| `label` | ラベル名 | `#xxx` |

`local`/`global`(宣言名)・`single`・`deftype`は他のカテゴリより狭く、対応するプレフィックス1種類のみを許容する。`callname`に`%xxx`が含まれるのは、構造体のメソッド値やクロージャーを保持したローカル変数をそのまま呼び出せるようにするため(8節参照)。

## 6. コンテナ型はいずれも`TYPE`系命令による事前宣言が必須

チャネル・スライス・マップ・構造体・関数型(クロージャー含む)は、対応する`TYPE`系命令(`CHTYPE`/`SLTYPE`/`STTYPE`/`MPTYPE`/`FNTYPE`)で`deftype`として事前宣言してから使う。型のインライン埋め込み(`^chan xxx`や`^[]xxx`のような複合形をその場で書くこと)は廃止されており、以降は宣言した`deftype`(`^xxx`のみの単純な形)を参照する。

```
CHTYPE	^IntChan	^int      // type IntChan chan int
SLTYPE	^IntSlice	^int      // type IntSlice []int
MPTYPE	^StrIntMap	^string	^int  // type StrIntMap map[string]int
```

`CHMAKE`/`SLMAKE`/`MPMAKE`は、事前宣言した`deftype`を使って`make`を呼び出し、既存の(`VAR`/`GVAR`で宣言済みの)変数へ`=`で代入する。`MPMAKE`はサイズ引数を取らない(`make(deftype)`)。

### スライス式(部分列の取得)

`SLICE single slice1 from to`という専用命令で、スライス・配列・文字列から部分列を取得する(`single = slice1[from:to]`)。`from`/`to`はどちらも`_`で省略可能で、省略時はGo側で空欄になる(`slice1[:to]`のように)。

## 7. 型表現(`^`)

型は全て`^`で始まる。要素部分にはプレフィックスを付けない裸の識別子を使う。以前存在した`chan`/`[]`/`map`のインライン形は廃止され、単純/セレクタ/ポインタ/配列の4系統×セレクタ有無の8パターンのみになっている。

```
^xxx                単純型
^xxx.xxx            セレクタ型
^*xxx                ポインタ型
^*xxx.xxx            ポインタ+セレクタ型
^[n]xxx              配列型(単純要素)
^[n]xxx.xxx          配列型(セレクタ要素)
^[n]*xxx             配列型(ポインタ要素)
^[n]*xxx.xxx         配列型(ポインタ+セレクタ要素)
```

`deftype`(`CHTYPE`/`SLTYPE`/`STTYPE`/`MPTYPE`/`FNTYPE`で宣言する名前、およびそれを参照する側)はこのうち単純型(`^xxx`)のみを許容する。配列は1次元固定長のみで、多次元配列はAMIVM-IR自体では表現しない(フロントエンド側で1次元に展開する前提)。

## 8. 構造体・ポインタ・メソッド呼び出しの扱い

構造体は`STTYPE`/`FIELD`/`ENDSTTYPE`で宣言し、`FSET`/`FGET`でフィールドの読み書きを直接行う(以前の設計にあった「中身の見えない不透明な値」という制約は撤廃されている)。

```
STTYPE	^Point
FIELD	>X	^int
FIELD	>Y	^int
ENDSTTYPE
```

ポインタは「`VAR`/`GVAR`でのポインタ型宣言(`^*xxx`)」「`ADDR`によるアドレス取得」「`PGET`/`PSET`によるデリファレンス(読み取り・書き込み両方)」の3命令で構成する。

### メソッド呼び出しのパターン

構造体の**メソッド呼び出し**(例: `file.Close()`)は、次の2段階で表現する。

1. `FNTYPE`で(レシーバーを除いた)メソッドの関数型を`deftype`として宣言する
2. `FGET`で、その構造体変数からメソッドを**値(Goのメソッド値)として取り出す**(`FGET single variable field`の`field`がメソッド名の場合、`single = variable.field`はメソッド値の取得になる)
3. 取り出したメソッド値を`%`(ローカル変数)に保持し、`CALL`の呼び出し対象としてそのまま使う(`callname`カテゴリに`%xxx`が含まれる理由)

```
FNTYPE	^CloseFn	:	^error

VAR	%f	^*os.File
VAR	%err	^error
VAR	%closeFn	^CloseFn
VAR	%closeErr	^error

CALL	%f	%err	:	?os.Open	"go.mod"
FGET	%closeFn	%f	>Close
CALL	%closeErr	:	%closeFn
```

## 9. 組み込み関数・型変換

`close`, `len`, `cap`, `panic`, `recover`等のGo組み込み関数、および型変換(キャスト)は専用命令を作らず、`CALL`(`callname`に`?close`や`?int`のような形)で表現する。

```
CALL	:	?close	%ch
CALL	%n1	:	?len	%sl
CALL	%x	:	?int	%y
```

## 10. クロージャー

`FNTYPE`でクロージャー変数の型(`deftype`)を宣言し、`CLOS`〜`ENDCLOS`で本体を組み立てて既存の`VAR`変数(`local`)に代入する。クロージャー引数は`&N`(`amivm_closure_paramN`)、外側の`FUNC`の`$N`/`%xxx`/`@xxx`は通常のGoのクロージャーと同様にそのまま参照(キャプチャ)できる。

```
FNTYPE	^BinOp	^int	^int	:	^int

VAR	%adder	^BinOp
CLOS	%adder	^int	^int	:	^int
	VAR	%sum	^int
	ADD	%sum	&1	&2
	RET	%sum
ENDCLOS
CALL	%result	:	%adder	1	2
```

`CLOS`は`FUNC`本体内にのみ出現し、ネスト不可。`LABEL`の直後に`SEL`/`CLOS`が来る場合の扱いは11節参照。

## 11. `LABEL`は常に`label: ;`(空文とセット)を生成する

`LABEL label`は、直後に何が続くかに関わらず**常に**`label: ;`(ラベル+空文)を生成する。ラベル自身は他の行と一切連動しない、独立した1行完結の命令である。

```
LABEL	#afterSend
SEL
CASERECV	%v	%ch	#gotval
DEFAULT	#nothing
ENDSEL
```

は次のように生成される。

```go
afterSend:
	;
	select {
	case v = <-ch:
		goto gotval
	default:
		goto nothing
	}
```

`LABEL`の直後に`SEL`/`CLOS`のような複数行ブロックが来ても、直後の通常の1行(`SET`等)が来ても、`ENDFUNC`/`ENDSEL`/`ENDCLOS`(ブロック終端)が来ても、`LABEL`自体の生成結果は変わらない。これにより「次の行が何であるかによって挙動を変える」先読み処理が一切不要になり、`LABEL`は`GOTO`や`SET`と同じ、通常の1行命令として扱える。

## 12. 実装パイプライン

```
IRテキスト
  → 空行・//コメント行を除去した上で、タブ区切りでトークナイズ+分類(プレフィックス記号から先頭でKindが確定する)
  → 命令名(先頭トークン)で緩く分岐 → 命令ごとにカテゴリと照合して厳密に検証
  → ast.File 組み立て(FUNC/SEL/CLOS/STTYPEのブロック抽出を含む)
  → format.Node でテキスト化
  → imports.Process で import解決
  → parser.ParseFile で再パース
  → go/types で型チェック
      - 未使用変数エラーのみ検出 → 変数名から所属関数を特定し、VAR宣言直後(ネストしたブロック内も再帰的に探索)に `_ = x` を挿入して再チェック(最大5回ループ)
      - それ以外のエラー → 即座に失敗として返す
  → os.WriteFile で出力先(-oで指定したパス。省略時はIRファイルパスの拡張子を.goに置き換えたパス)へ書き出し
```

`amivm`コマンドの責務はここまで(Goソースファイルの出力)で完結する。生成したファイルを実行ファイルにする`go build`は、amivm自身は実行しない別の後続作業である(パイプライン図の「AMIVM → Goコード」と「Goコード → 実行ファイル」が別工程であることに対応する)。

```
amivm <IRファイルパス> [-o <出力ファイルパス>] [-v]
```

`-o`省略時の出力先は、IRファイルパスの拡張子を`.go`に置き換えたパス(拡張子が無ければ`.go`を付け足したパス)になる。`-v`を付けると元のIR・型チェックの過程(未使用変数の自己修復ログ含む)・最終的な生成コード・完了メッセージを標準出力に表示する。付けない場合、成功時は何も出力しない(ファイル読み込み失敗・パースエラー・型チェック失敗などのエラーは`-v`の有無に関わらず常に出力される)。

- 意味の正しさ(型整合性、未定義識別子、メソッド存在チェックなど)は`go/types`に全面的に委ね、AMIVM側で独自の検証ロジックは持たない
- IR行番号と生成Goコードのエラー行を対応付ける仕組みは、当面の実装では省略する。ただし4節の命名規則により、エラーメッセージから「どの関数由来か」は分かるようになっている

## 13. 過去の設計判断からの変更点(経緯メモ)

### CLIから`go build`を切り離す変更

以前は`go run main.go <IRファイルパス>`という呼び出し前提で、内部で`go build -o output ...`まで実行し実行ファイルを生成していた。しかし当初のパイプライン図(`amivm_spec.md`/`CLAUDE.md`)が示す通り「AMIVM → Goコード」と「Goコード → 実行ファイル」は本来別工程であり、amivmコマンド自身が実行ファイル生成まで担う必然性は無い。そこで`amivm <IRファイルパス> [-o <出力ファイルパス>] [-v]`というコマンド仕様に変更し、`go build`の実行そのものを削除した。出力ファイルパスを明示的に選べるようにする`-o`、および進捗表示を選べる`-v`もこのタイミングで導入した。

### `LABEL`を常に`label: ;`にする変更

以前は「`LABEL`の直後が`SEL`/`CLOS`(複数行ブロック)の場合だけ空文を挟む」という条件付きの実装だった。`LABEL`はどんな行が続くかによって挙動を変える必要はなく、**常に**`label: ;`を生成する方が仕様として単純であり、パーサー側も先読み処理が丸ごと不要になる(`LABEL`が他の1行命令と全く同じ扱いになる)と判断し、`amivm_spec.md`(当時は`IR.txt`)・実装の両方をこの形に統一した。

### 第1版からの変更(識別子の複合形廃止・命令の大幅追加)

- **識別子(`$`/`%`/`@`)に埋め込まれていた添字・スライス・デリファレンス・アドレス取得の複合形を全廃**。`ASET`/`AGET`/`PSET`/`PGET`/`ADDR`/`SLICE`という専用命令に分離した。これにより識別子側のKindが`$N`/`&N`/`%xxx`/`@xxx`/`@xxx.yyy`の5種類だけになり、Kind総数が大幅に減った
- **`&`のプレフィックス意味を「アドレス取得」から「クロージャー引数」に変更**。アドレス取得は`ADDR`命令に統合された
- **チャネル・スライスに加え、マップ・構造体・関数型(クロージャー)も`TYPE`系命令(`CHTYPE`/`SLTYPE`/`STTYPE`/`MPTYPE`/`FNTYPE`)による事前宣言必須の設計に統一**。型のインライン埋め込み(`^chan xxx`等)を廃止した
- **構造体を「不透明な値」として扱う制約を撤廃**し、`FSET`/`FGET`によるフィールドの直接読み書きを追加。メソッド呼び出しは`FNTYPE`+`FGET`でメソッド値を取り出す2段階パターンに統一した
- **`CALL`/`FUNC`/`FNTYPE`/`CLOS`に`:`区切りを導入**。以前の「`!`/`?`で始まる最初のトークンを走査して呼び出し対象を探す」方式から変更した
- **`CASE SEND`/`CASE RECV`を`CASESEND`/`CASERECV`の1語に統合**
- **`//`コメント行のサポートを追加**
- **旧`ch`カテゴリ(チャネル/スライス共有)を`single`に統合**し、意味を「単一左辺、チャネル/スライス/マップ/構造体変数」に整理した
- **`CLOS`/`ENDCLOS`(クロージャー)を追加**。`&N`という専用プレフィックスの引数を持つ

### それ以前の変更点

- **チャネルを宣言不可としていた制約を撤廃**。`:=`を使うことによる変数シャドーイングのバグをきっかけに、「全ての変数は宣言→=で代入」という一貫したルールに統一する方が構造的に安全だと判断した
- **トークンの分類(Kind)に、値だけでなく型の形状も含めるよう統一**
- **`CONST`/`CONV`/`PRINT`を削除**。`CONST`はフロントエンド側のリテラル埋め込みで代替、`CONV`はGoの型変換`T(v)`が`ast.CallExpr`と同一構造のため`CALL`で表現可能、`PRINT`は動作確認用の暫定命令で`CALL`(`?fmt.Println`)により代替可能と判断した
- **スライスを導入**。配列は型に長さが埋め込まれ関数の引数・戻り値に不向きなため
- **全識別子にプレフィックス記号を導入**。以前`@`(AMIVM関数)/`?`(Go関数)にのみ導入していた「先頭記号で種別を明示する」考え方を、変数・型・ラベル・フィールドまで含めた全カテゴリに拡張した
- **`VAR`(関数内)と`GVAR`(関数外)を明確に別命令として分離**
- **未使用変数の`_ = x`挿入方式を、AST全体検索から命名規則ベースの文字列分割に変更**。挿入位置もVAR宣言の直後に変更し、`missing return`エラーの誘発を防いだ
