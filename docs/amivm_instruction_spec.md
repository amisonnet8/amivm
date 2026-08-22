# AMIVM IR命令セット仕様

本ドキュメントはコンパイラ本体の実装に対応する解説版であり、**唯一の正確な仕様は`amivm_spec.md`**(プロジェクトルート)である。本ドキュメントと`amivm_spec.md`が矛盾する場合は`amivm_spec.md`を優先すること。

## 0. トークンの区切り文字・コメント

各命令のトークンは**タブ文字**で区切る。文字列リテラル(`"hello world"`のようにスペースを含むもの)が誤って複数トークンに分割されるのを避けるため。行頭の空白カラム群(インデント用の連続タブ)は無視し、それ以外のカラムはトリムせずそのまま使う。

`//`で始まる行はコメント行として無視する(行全体がコメントの場合のみ対応。行末コメントは非対応)。

## 1. 識別子のプレフィックス体系

全ての識別子(変数・型・関数・ラベル・構造体フィールド)は、種別を表す記号を先頭に持つ。プレフィックスなしの裸の識別子が出現するのは、型宣言(`^`)や構造体フィールド宣言(`FIELD`)の中の要素名部分のみ。

| プレフィックス | 意味 | Go生成名 |
|---|---|---|
| `$` | 関数引数(パラメータ) | `$N` → `<関数名>_amivm_function_paramN` |
| `&` | クロージャー引数 | `&N` → `amivm_closureL_paramN`(`L`は自分がいる`CLOS`階層。関数名による修飾なし)。`&L-N`で階層`L`を明示指定できる |
| `%` | 関数内変数名 | `%xxx` → `<関数名>_amivm_function_xxx` |
| `@` | 関数外(パッケージレベル)変数名 | `@xxx` はそのまま、`@pkg.xxx` は他パッケージの値参照 |
| `^` | 型名 | `^xxx` → `xxx`(7節参照) |
| `>` | 構造体フィールド名 | `>xxx` → `xxx` |
| `!` | AMIVM内定義関数名 | `!xxx` → `xxx_amivm_function`、`!main` → `main`のまま |
| `?` | Go関数名 | そのまま使う(`?xxx.yyy`はパッケージセレクタ) |
| `#` | ラベル名 | そのまま使う(Goのラベル構文にプレフィックスは含めない) |

宣言側にも参照側と同じプレフィックスを要求する(`VAR`は`%`のみ、`GVAR`は`@`のみ)。これにより「関数内で`@`を使って宣言する」「トップレベルで`%`を使う」といった文脈のミスマッチを、宣言の時点でパースエラーとして検出できる。

識別子(`$`/`&`/`%`/`@`)には添字・スライス・デリファレンス・アドレス取得を埋め込んだ複合形は存在しない。それらは全て専用命令(`ASET`/`AGET`/`PSET`/`PGET`/`ADDR`/`SLICE`)に分離されている。そのため識別子側のKindは`$N`/`&N`/`%xxx`/`@xxx`/`@xxx.yyy`の5種類のみで、`@xxx.yyy`(他パッケージ参照)以外は全カテゴリで共通に扱える(`&L-N`は`&N`と同じKindの別表記であり、Kindの種類数には数えない。10節参照)。

## 2. トップレベルの制約

関数の外(トップレベル)に置けるのは`GVAR`・`FUNC`・`CHTYPE`・`SLTYPE`・`STTYPE`・`MPTYPE`・`FNTYPE`のみ。`GVAR`はGoのパッケージレベル変数になる。`FUNC`のみ関数本体を持つブロックとして関数外に置け、それ以外の命令(演算・制御フロー等)は`FUNC`〜`ENDFUNC`の中でのみ使用できる。`FUNC`・`STTYPE`はネスト不可。`IF`・`LOOP`・`CLOS`・`SEL`はいずれもネストできる(互いの本体の中に任意の組み合わせ・任意の深さで書ける。10節・11節・12節参照)。

## 3. 命令一覧

| IR命令(タブ区切り) | Go生成コード |
|---|---|
| `VAR local type1` | `var local type1`(関数内のみ) |
| `GVAR global type1` | `var global type1`(関数外のみ) |
| `SET single1 value1` | `single1 = value1` |
| `ASET single1 whole value1` | `single1[whole] = value1` |
| `AGET single1 variable whole` | `single1 = variable[whole]` |
| `PSET single1 value1` | `*single1 = value1` |
| `PGET single1 variable` | `single1 = *variable` |
| `ADDR single1 variable (point)` | `point`無し: `single1 = &variable` / `point`が`>xxx`: `single1 = &variable.point` / それ以外: `single1 = &variable[point]` |
| `ADD single1 number1 number2` | `single1 = number1 + number2` |
| `SUB single1 number1 number2` | `single1 = number1 - number2` |
| `MUL single1 number1 number2` | `single1 = number1 * number2` |
| `DIV single1 number1 number2` | `single1 = number1 / number2` |
| `MOD single1 integer1 integer2` | `single1 = integer1 % integer2` |
| `BAND single1 integer1 integer2` | `single1 = integer1 & integer2` |
| `BOR single1 integer1 integer2` | `single1 = integer1 \| integer2` |
| `BXOR single1 integer1 integer2` | `single1 = integer1 ^ integer2` |
| `BCLEAR single1 integer1 integer2` | `single1 = integer1 &^ integer2` |
| `BNOT single1 integer1` | `single1 = ^integer1` |
| `SHL single1 integer1 whole` | `single1 = integer1 << whole` |
| `SHR single1 integer1 whole` | `single1 = integer1 >> whole` |
| `AND single1 boolean1 boolean2` | `single1 = boolean1 && boolean2` |
| `OR single1 boolean1 boolean2` | `single1 = boolean1 \|\| boolean2` |
| `NOT single1 boolean1` | `single1 = !boolean1` |
| `EQ single1 value1 value2` | `single1 = value1 == value2` |
| `NEQ single1 value1 value2` | `single1 = value1 != value2` |
| `LT single1 ordered1 ordered2` | `single1 = ordered1 < ordered2` |
| `LTE single1 ordered1 ordered2` | `single1 = ordered1 <= ordered2` |
| `GT single1 ordered1 ordered2` | `single1 = ordered1 > ordered2` |
| `GTE single1 ordered1 ordered2` | `single1 = ordered1 >= ordered2` |
| `CONCAT single1 slice1 slice2 ...` | `single1 = slice1 + slice2 ...` |
| `LABEL label` | `label: ;` |
| `GOTO label` | `goto label` |
| `IF boolean1` | `if boolean1 {` |
| `ELIF boolean1` | `} else if boolean1 {` |
| `ELSE` | `} else {` |
| `ENDIF` | `}`(`IF`終端) |
| `LOOP` | `for {` |
| `BREAK` | `break` |
| `CONTINUE` | `continue` |
| `ENDLOOP` | `}`(`LOOP`終端) |
| `ASSERT multi1 (multi2) variable type1` | `multi1(, multi2) = variable.(type1)` |
| `FUNC defname type1 type2 ... : type3 type4 ...` | `func defname(param1 type1, param2 type2 ...) (type3, type4 ...) {` |
| `RET value1 value2 ...` | `return value1, value2 ...` |
| `ENDFUNC` | `}`(`FUNC`終端) |
| `CALL multi1 multi2 ... : callname value1 value2 ...` | `multi1, multi2 ... = callname(value1, value2 ...)`(`multi1`が無ければ式文として呼ぶだけ) |
| `DEFER callname value1 value2 ...` | `defer callname(value1, value2 ...)` |
| `SPAWN callname value1 value2 ...` | `go callname(value1, value2 ...)` |
| `CHTYPE deftype type1` | `type deftype chan type1`(関数外) |
| `CHMAKE single1 deftype whole` | `single1 = make(deftype, whole)` |
| `CHSEND single1 value1` | `single1 <- value1` |
| `CHRECV multi1 (multi2) single1` | `multi1(, multi2) = <-single1` |
| `SEL` | `select {` |
| `CASESEND single1 value1` | `case single1 <- value1:`(`SEL`内。以降`ENDSEL`か次のケースまでが本体) |
| `CASERECV multi1 (multi2) single1` | `case multi1(, multi2) = <-single1:`(`SEL`内。同上) |
| `DEFAULT` | `default:`(`SEL`内。同上) |
| `ENDSEL` | `}`(`SEL`終端) |
| `SLTYPE deftype type1` | `type deftype []type1`(関数外) |
| `SLMAKE single1 deftype whole` | `single1 = make(deftype, whole)` |
| `SLICE single1 slice1 from to` | `single1 = slice1[from:to]` |
| `STTYPE deftype` | `type deftype struct {`(関数外) |
| `FIELD field type1` | `field type1`(`STTYPE`内) |
| `ENDSTTYPE` | `}`(`STTYPE`終端) |
| `FSET single1 field value1` | `single1.field = value1` |
| `FGET single1 variable field` | `single1 = variable.field` |
| `MPTYPE deftype type1 type2` | `type deftype map[type1]type2`(関数外) |
| `MPMAKE single1 deftype` | `single1 = make(deftype)` |
| `MSET single1 value1 value2` | `single1[value1] = value2` |
| `MGET multi1 (multi2) single1 value1` | `multi1(, multi2) = single1[value1]` |
| `MPKEYS single1 single2` | `single1 = slices.Collect(maps.Keys(single2))` |
| `FNTYPE deftype type1 type2 ... : type3 type4 ...` | `type deftype func(type1, type2 ...) (type3, type4 ...)`(関数外) |
| `CLOS single1 type1 type2 ... : type3 type4 ...` | `single1 = func(amivm_closureL_param1 type1, amivm_closureL_param2 type2 ...) (type3, type4 ...) {` |
| `ENDCLOS` | `}`(`CLOS`終端) |

`ADD`は数値演算専用(常に2オペランド)。文字列連結は`CONCAT`(可変長引数)で行う。キャスト(型変換)・組み込み関数(`close`/`len`/`cap`等)は専用命令を持たず、`CALL`に統合している(9節参照)。型アサーション(Goの`x.(T)`)だけは`CALL`に含めず`ASSERT`という専用命令にしている(13節参照)。

`ADDR`の`point`は省略可能な第3引数で、`&variable`単体だけでなく`&variable.field`(構造体フィールドのアドレス)・`&variable[index]`(スライス/配列要素のアドレス)も表現できる。`point`が`>xxx`(フィールド名)かそれ以外かで生成先が分岐する。`&variable[point]`はスライス/配列専用で、`variable`がmapの場合は文法上は通っても`go/types`が「mapの要素はアドレス取得できない」というエラーを返す(AMIVM側では検証しない。9節の設計方針どおり)。

`MPKEYS`はmapを走査する手段が無い(AMIVM-IRに`for range`に相当する命令が無く、mapのキー一覧を得る手段がGoの`for k := range m`しか無い)という不備を解消するために追加した。`slices.Collect(maps.Keys(m))`というGo 1.23+の標準ライブラリの組み合わせに展開され、importはgoimportsが自動解決する(`slices`/`maps`とも標準ライブラリなので、`-i`/`--import`オプションは不要)。

### `:`(コロン)区切り

`CALL`・`FUNC`・`FNTYPE`・`CLOS`の4命令は、トークン列の中に必ず1個の`:`を含み、それを境に「代入先・関数名」の側と「呼び出し対象・パラメータ列/戻り値型」の側を分割する。

- `FUNC`/`FNTYPE`: `:`より前がパラメータ型列、後が戻り値型列
- `CALL`: `:`より前が代入先(`multi`列、0個以上)、後が呼び出し対象+引数列
- `CLOS`: 代入先(`single1`)の次から`:`より前がパラメータ型列、後が戻り値型列

`:`が無い、または2個以上あるとパースエラーになる(パラメータ・戻り値・代入先が0個の場合でも`:`自体は省略できない)。`DEFER`/`SPAWN`は`callname value1 value2 ...`のみで`:`を使わない。

### `SEL`/`CASESEND`/`CASERECV`/`DEFAULT`/`ENDSEL`

`CASESEND`/`CASERECV`は(旧`CASE SEND`/`CASE RECV`から)1語化されている。`CHRECV`/`CASERECV`の`(multi2)`は省略可能。省略時は1つ、指定時は2つ(値, ok)の代入になる。`SEL`は`FUNC`内、または`IF`/`LOOP`/`CLOS`/`SEL`の本体内に出現する(ネストできる。12節参照)。

`CASESEND`/`CASERECV`/`DEFAULT`はもはや`label`を取らない。Goの`select`の`case`/`default`節がそうであるように、次の`CASESEND`/`CASERECV`/`DEFAULT`または`ENDSEL`が現れるまでの範囲を自分の本体として持つブロックになっており、本体には`VAR`宣言を含む任意の命令(`IF`/`LOOP`/`CLOS`/`SEL`のネストを含む)を書ける。この変更の経緯は16節参照。

### 変数は常に「宣言 → `=`で代入」に統一する

チャネル・スライス・マップ・構造体変数も含め、全ての変数は`VAR`/`GVAR`で事前に宣言してから代入する形に統一する。`CHMAKE`/`SLMAKE`/`MPMAKE`は新規宣言(`:=`)ではなく、既存変数への代入(`=`)として扱う。同様に`CASERECV`のような受信処理も含め、あらゆる代入命令が一貫して`=`(既存変数への代入)を使う設計になっており、`:=`によるスコープ内シャドーイングが構造的に発生しない(過去にこの点で実際にバグを踏んだため、設計として固定している)。

## 4. VAR/GVAR/CLOSの命名規則(未使用変数対応との関連)

関数内変数(`VAR`, `%`)は、Go側の実際の変数名を`<関数名>_amivm_function_<変数名>`という形にする。パラメータ(`$N`)も同様に`<関数名>_amivm_function_paramN`とする。`main`関数も例外なくこの規則に従う(関数宣言自体の名前は`main`のまま、変数名prefixとしては`main_amivm_function_`を使う)。

クロージャー引数(`&N`/`&L-N`)は、関数名による修飾を持たず`amivm_closureL_paramN`という固定名になる(`L`はその`CLOS`のネスト深さ、`FUNC`直下を1として数える)。`CLOS`がネストできない旧仕様では常に`L=1`(実質`amivm_closure_paramN`)で問題なかったが、`CLOS`をネストできるようにしたことで、内側の`CLOS`が外側と同じ`amivm_closure_paramN`を使うと外側のパラメータをGoの通常のスコープルールでシャドーイングしてしまい、外側のパラメータを内側から参照する手段が失われる。これを避けるため、パラメータ名に階層`L`を埋め込んで各階層を別名にした。`CLOS`は無名の`func`リテラルとして展開されるため、兄弟関係にある(親子関係にない)`CLOS`同士が同じ`amivm_closureL_paramN`を使っても、Goの別ブロックスコープとして衝突しない。`CLOS`本体内で`VAR`宣言した変数(`%xxx`)は、`CLOS`を包む外側の`FUNC`の名前で修飾される(`CLOS`自体は無名なので専用の名前空間を持たない。ネストしていても、修飾に使われる「外側の`FUNC`」は常に同じ)。

パッケージレベル変数(`GVAR`, `@`)は、命名規則を変更せずそのままの変数名を使う。Goは未使用のパッケージレベル変数をコンパイルエラーにしないため、命名規則による対応が不要なことに対応している。

この命名規則により、`go/types`が返す`declared and not used`エラーメッセージの変数名から、`_amivm_function_`という固定マーカー文字列で分割するだけで所属関数を特定できる。

未使用変数の`_ = x`挿入位置は、該当する`VAR`宣言文の直後にする。関数末尾に追加すると、戻り値のある関数で最後の文が`return`でなくなり`missing return`エラーを誘発するため。挿入先の探索は関数のトップレベル文だけでなく、`IF`本体・`SEL`の各`CommClause`・`CLOS`(`func`リテラル)の内部まで再帰的に辿る(`CLOS`はネストしたブロックスコープを作るため、これを省略すると`CLOS`内で未使用になった変数を救済できない)。

## 5. オペランドカテゴリ

各カテゴリの識別子側の許容範囲は、`$N`/`&N`/`%xxx`/`@xxx`に加えて`@xxx.yyy`(他パッケージ参照)を含むかどうかで大きく2系統に分かれる。`&N`を許容するカテゴリは全て`&L-N`(階層`L`を明示する形)も同様に許容する(両者は同じKindの表記違いにすぎないため。4節参照)。

- **フル系**(`@xxx.yyy`含む): `whole`/`integer`/`number`/`boolean`/`from`・`to`/`slice`/`ordered`/`value`/`variable`
- **単純系**(`@xxx.yyy`含まない): `single1`/`single2`/`multi`(`multi`はさらに`_`を許容)

| カテゴリ | 意味 | 許容形式 |
|---|---|---|
| `local` | `VAR`宣言名(関数内) | `%xxx` |
| `global` | `GVAR`宣言名(関数外) | `@xxx` |
| `single1 single2` | 単一左辺・チャネル/スライス/マップ/構造体変数の参照(`CLOS`の代入先も含む) | `$N` / `&N` / `%xxx` / `@xxx` |
| `multi1 multi2` | 複数左辺(`CALL`/`CHRECV`/`CASERECV`/`MGET`の代入先) | `single1`/`single2`の形式 + `_` |
| `variable` | 読み取り専用の変数参照(`AGET`/`PGET`/`ADDR`/`FGET`の読み出し元) | `$N` / `&N` / `%xxx` / `@xxx` / `@xxx.yyy` |
| `field` | 構造体フィールド名 | `>xxx` |
| `type1 type2 type3 type4` | 型表現(7節) | `^xxx`系8パターン |
| `deftype` | `TYPE`系命令で宣言・参照する定義型名 | `^xxx`(単純形のみ) |
| `whole` | 0以上の整数(添字・シフト量・サイズ等) | フル系識別子 + `0`,`1234` / `'A'` |
| `point` | `ADDR`の第3引数(フィールド/添字の対象) | `whole`の形式 + `>xxx` |
| `from to` | `SLICE`の範囲指定 | `whole`の形式 + `_`(省略。Go側では空欄になる) |
| `integer1 integer2` | 整数オペランド | `whole`の形式 + `-1234` |
| `number1 number2` | 数値オペランド | `integer`の形式 + `123.4`,`1.23e4` |
| `boolean1 boolean2` | 真偽値オペランド | フル系識別子 + `true`,`false` |
| `slice1 slice2` | スライス・文字列(`CONCAT`/`SLICE`の対象) | フル系識別子 + `"ABC"` |
| `ordered1 ordered2` | 順序比較可能な値 | フル系識別子 + `0`,`1234`,`-1234` / `123.4`,`1.23e4` / `"ABC"` / `'A'` |
| `value1 value2` | 値全般(最も緩い) | `ordered`の形式 + `true`,`false` / `nil` / `!xxx` / `?xxx` / `?xxx.yyy`(関数そのものを、呼び出さずに値として渡す) |
| `defname` | AMIVM内で定義する関数名 | `!xxx` / `!main` |
| `callname` | 呼び出し対象 | `!xxx` / `!main` / `?xxx` / `?xxx.yyy` / `%xxx`(関数値・メソッド値・クロージャーを保持するローカル変数) / `@xxx`(同、パッケージレベル変数) / `$N` / `&N`(パラメータ・クロージャー引数として受け取った関数値をそのまま呼び出す) |
| `label` | ラベル名 | `#xxx` |

`local`/`global`(宣言名)・`deftype`は他のカテゴリより狭く、対応するプレフィックス1種類のみを許容する。`CLOS`の代入先は`single1`をそのまま流用しており、専用カテゴリは持たない(`&N`/`&L-N`でクロージャー引数を代入し直すケースも含め、`single1`が許容する形式がそのままCLOSの代入先として妥当なため。10節参照)。`callname`に`%xxx`/`@xxx`が含まれるのは、構造体のメソッド値やクロージャーを保持したローカル変数・パッケージレベル変数をそのまま呼び出せるようにするため(8節参照)。`$N`/`&N`も同じ理由で含まれている。クロージャーをパラメータ(`$N`)やクロージャー引数(`&N`)として受け取った側でそのまま呼び出したいケースに対応するためで、以前はこれが抜けており「クロージャーを引数で渡してもそのまま呼び出せない」という仕様上の不備だった。

## 6. コンテナ型はいずれも`TYPE`系命令による事前宣言が必須

チャネル・スライス・マップ・構造体・関数型(クロージャー含む)は、対応する`TYPE`系命令(`CHTYPE`/`SLTYPE`/`STTYPE`/`MPTYPE`/`FNTYPE`)で`deftype`として事前宣言してから使う。型のインライン埋め込み(`^chan xxx`や`^[]xxx`のような複合形をその場で書くこと)は廃止されており、以降は宣言した`deftype`(`^xxx`のみの単純な形)を参照する。

```
CHTYPE	^IntChan	^int      // type IntChan chan int
SLTYPE	^IntSlice	^int      // type IntSlice []int
MPTYPE	^StrIntMap	^string	^int  // type StrIntMap map[string]int
```

`CHMAKE`/`SLMAKE`/`MPMAKE`は、事前宣言した`deftype`を使って`make`を呼び出し、既存の(`VAR`/`GVAR`で宣言済みの)変数へ`=`で代入する。`MPMAKE`はサイズ引数を取らない(`make(deftype)`)。

### スライス式(部分列の取得)

`SLICE single1 slice1 from to`という専用命令で、スライス・配列・文字列から部分列を取得する(`single1 = slice1[from:to]`)。`from`/`to`はどちらも`_`で省略可能で、省略時はGo側で空欄になる(`slice1[:to]`のように)。

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
2. `FGET`で、その構造体変数からメソッドを**値(Goのメソッド値)として取り出す**(`FGET single1 variable field`の`field`がメソッド名の場合、`single1 = variable.field`はメソッド値の取得になる)
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

`FNTYPE`でクロージャー変数の型(`deftype`)を宣言し、`CLOS`〜`ENDCLOS`で本体を組み立てて既存の変数(`single1`。`$N`/`%xxx`/`@xxx`/`&N`/`&L-N`のいずれか。事前に`VAR`/`GVAR`で宣言済み、関数パラメータとして存在する、またはネストしている場合は外側`CLOS`のクロージャー引数)に代入する。クロージャー引数は`&N`(自分がいる`CLOS`階層の`N`番目。実際のGo名は`amivm_closureL_paramN`)、外側の`FUNC`の`$N`/`%xxx`/`@xxx`は通常のGoのクロージャーと同様にそのまま参照(キャプチャ)できる。

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

代入先は`%xxx`(ローカル変数)に限らない。関数パラメータ(`$N`)をクロージャーで差し替えたり、パッケージレベル変数(`@xxx`)にクロージャーを代入することもできる(後者はそのまま`callname`として`@xxx`を使って呼び出せる)。`test_ir/15_closure.ir`に実例がある。

```
CLOS	@globalAdder	^int	^int	:	^int
	VAR	%s	^int
	ADD	%s	&1	&2
	RET	%s
ENDCLOS
CALL	%viaGlobal	:	@globalAdder	10	20
```

### `CLOS`のネスト

`CLOS`は他のブロック命令(`FUNC`/`STTYPE`/`SEL`)と違い、`CLOS`本体の中にさらに`CLOS`をネストできる(クロージャーを返すクロージャー、いわゆるカリー化を表現するため)。ネストの深さ`L`は`FUNC`直下に置かれた最初の`CLOS`を1とし、ネストするごとに1ずつ増える。

クロージャー引数`&N`は「自分がいる`CLOS`階層のN番目」を指す。内側の`CLOS`から外側の`CLOS`のクロージャー引数を参照したい場合は、階層を明示する`&L-N`を使う。

```
FNTYPE	^Adder	^int	:	^int
FNTYPE	^Curry	^int	:	^Adder

FUNC	!curry	:	^Curry
	VAR	%f	^Curry
	CLOS	%f	^int	:	^Adder
		VAR	%g	^Adder
		CLOS	%g	^int	:	^int
			VAR	%s	^int
			ADD	%s	&1	&1-1
			RET	%s
		ENDCLOS
		RET	%g
	ENDCLOS
	RET	%f
ENDFUNC
```

外側の`CLOS`(階層1)は`%f`に代入され、`&1`はその1番目のパラメータを指す。内側の`CLOS`(階層2)は`%g`に代入され、その本体では`&1`が「自分(階層2)の1番目」、`&1-1`が「階層1の1番目」を指す(`curry(a)(b) = a + b`)。`%f`/`%g`のような代入先自体も、内側の`CLOS`から見れば外側の`CLOS`本体で`VAR`宣言された変数なので、通常のGoクロージャーと同じくキャプチャして参照できる(`&L-N`はあくまでクロージャー引数専用の記法で、代入先の変数名を参照する場合は通常どおり`%xxx`を使う)。

ネストしたクロージャー引数を区別せずに全て`amivm_closure_paramN`という固定名にしてしまうと、内側の`CLOS`が外側と同じパラメータ番号を使った場合にGoの通常のスコープルールで外側がシャドーイングされ、外側のパラメータが内側から参照できなくなる。これを避けるため、Go側の実名に階層`L`を埋め込んでいる(4節参照)。

`CLOS`は`FUNC`本体内、または`IF`/`LOOP`/`CLOS`/`SEL`の本体内にのみ出現する。`LABEL`の直後に`SEL`/`CLOS`が来る場合の扱いは14節参照。

## 11. 条件分岐(IF)

`IF`〜`ENDIF`は、Goの`if`/`else if`/`else`チェーンに対応するブロック構造。以前の単一行`IF boolean1 label`(`if boolean1 { goto label }`という条件付き`goto`)を置き換える形で導入された(経緯は16節参照。旧仕様との後方互換は無い)。

```
IF	boolean1
	// 本体1
ELIF	boolean1
	// 本体2
ELSE
	// 本体3
ENDIF
```

`ELIF`は0個以上、`ELSE`は0個または1個(書くなら`ELIF`列より後・`ENDIF`の直前)を許す。この形は、Goの`ast.IfStmt`が`Else`フィールドに`*ast.IfStmt`(else-ifの連鎖)か`*ast.BlockStmt`(最終`else`)のどちらか一方しか持てないという構造にそのまま対応しており、`ELSE`の後に`ELIF`や別の`ELSE`が続くことを許すと、そもそも妥当なGo ASTを組み立てられない。そのためこの並び順の違反は、`go/types`に投げる前の構文解析の時点で(数少ない例外として)AMIVM側が検証する。

実装(`parseIfChain`)は、`IF`/`ELIF`1行分の条件+本体を読み、本体の終端が`ELIF`ならその行を新たな`IF`行と同じ扱いで自分自身へ再帰させて`Else`に据え、`ELSE`ならその本体を`*ast.BlockStmt`として`Else`に据え、`ENDIF`ならそこで打ち切る、という前方再帰で書かれている。本体の走査自体は`IF`/`LOOP`/`CLOS`/`SEL`いずれのネストも許す`parseBody`をそのまま使う。

## 12. ループ(LOOP)

`LOOP`〜`ENDLOOP`はGoの無限`for {}`に対応するブロック構造。`AMIVM-IR`には条件式付きループ専用の命令は無く、`while`相当の挙動は`LOOP`の中で`IF`と`BREAK`を組み合わせて表現する。

```
LOOP
	IF	boolean1
		BREAK
	ENDIF
	// ループ本体
ENDLOOP
```

`BREAK`/`CONTINUE`はそれぞれGoの無名`break`/`continue`にそのまま対応し、常に自分を直接囲む最も内側の`LOOP`に対して働く。ラベル付き`break`/`continue`に相当する機能は無い。`LOOP`の外で`BREAK`/`CONTINUE`を使った場合、AMIVM側では構文チェックを行わない(`IF`の`ELSE`順序チェックとは違い、`ast.BranchStmt{Tok: token.BREAK}`は`LOOP`の有無に関わらず組み立てられてしまうため)。生成したGoコードに対して`go/types`が「break is not in a loop」のようなエラーを返す。意味の正しさの検証を`go/types`に委ねる設計方針どおりの割り切りである。

`SEL`もGoの`select`と同様に`break`の対象になれる(9節参照)ため、`LOOP`を伴わずに`SEL`のケース本体へ書いた`BREAK`は、その`SEL`自体を抜ける挙動になる。これもGoの挙動をそのまま踏襲しているだけで、AMIVM側で特別な分岐は無い。

## 13. 型アサーション(ASSERT)

`ASSERT multi1 (multi2) variable type1`は、Goの型アサーション`variable.(type1)`に対応する。`CHRECV`/`CASERECV`/`MGET`と同じ`multi1 (multi2)`パターンを踏襲しており、`multi2`省略時は失敗すると`panic`する単一形(`multi1 = variable.(type1)`)、指定時は2つ目が`ok`値になり失敗しても`panic`しない形(`multi1, multi2 = variable.(type1)`)になる。

```
VAR	%x	^any
VAR	%n	^int
VAR	%s	^string
VAR	%ok	^bool

SET	%x	42
ASSERT	%n	%x	^int
ASSERT	%s	%ok	%x	^string
```

`variable`・`type1`はいずれも既存のカテゴリ(`variable`・`type`)をそのまま使えるため、`ASSERT`のために新設したカテゴリ・Kindは無い。キャスト(型変換)や`close`/`len`のような組み込み関数は`CALL`に統合しているが(9節参照)、型アサーションは`T(v)`という関数呼び出しに似た形を取らない(`v.(T)`という別構文の`ast.TypeAssertExpr`になる)ため、`CALL`には含めず専用命令にした。

## 14. `LABEL`は常に`label: ;`(空文とセット)を生成する

`LABEL label`は、直後に何が続くかに関わらず**常に**`label: ;`(ラベル+空文)を生成する。ラベル自身は他の行と一切連動しない、独立した1行完結の命令である。

```
LABEL	#afterSend
SEL
CASERECV	%v	%ch
	SET	%result	"got"
DEFAULT
	SET	%result	"empty"
ENDSEL
```

は次のように生成される。

```go
afterSend:
	;
	select {
	case v = <-ch:
		result = "got"
	default:
		result = "empty"
	}
```

`LABEL`の直後に`SEL`/`CLOS`/`IF`/`LOOP`のような複数行ブロックが来ても、直後の通常の1行(`SET`等)が来ても、`ENDFUNC`/`ENDSEL`/`ENDCLOS`/`ENDIF`/`ENDLOOP`(ブロック終端)が来ても、`LABEL`自体の生成結果は変わらない。これにより「次の行が何であるかによって挙動を変える」先読み処理が一切不要になり、`LABEL`は`GOTO`や`SET`と同じ、通常の1行命令として扱える。

## 15. 実装パイプライン

```
IRテキスト
  → 空行・//コメント行を除去した上で、タブ区切りでトークナイズ+分類(プレフィックス記号から先頭でKindが確定する)
  → 命令名(先頭トークン)で緩く分岐 → 命令ごとにカテゴリと照合して厳密に検証
  → ast.File 組み立て(FUNC/SEL/CLOS/STTYPE/IF/LOOPのブロック抽出を含む)
  → format.Node でテキスト化
  → imports.Process で import解決
  → parser.ParseFile で再パース
  → go/packages(typeCheck)で型チェック
      - 未使用変数エラーのみ検出 → 変数名から所属関数を特定し、VAR宣言直後(ネストしたブロック内も再帰的に探索)に `_ = x` を挿入して再チェック(最大5回ループ)
      - それ以外のエラー → 即座に失敗として返す
  → os.WriteFile で出力先(-o/--outputで指定したパス。省略時はIRファイルパスの拡張子を.goに置き換えたパス)へ書き出し
```

`amivm`コマンドの責務はここまで(Goソースファイルの出力)で完結する。生成したファイルを実行ファイルにする`go build`は、amivm自身は実行しない別の後続作業である(パイプライン図の「AMIVM → Goコード」と「Goコード → 実行ファイル」が別工程であることに対応する)。

```
amivm <IRファイルパス> [-o|--output <出力ファイルパス>] [-v|--verbose] [-i|--import <名前>=<importパス>]...
```

`-o`/`--output`省略時の出力先は、IRファイルパスの拡張子を`.go`に置き換えたパス(拡張子が無ければ`.go`を付け足したパス)になる。`-v`/`--verbose`を付けると元のIR・型チェックの過程(未使用変数の自己修復ログ含む)・最終的な生成コード・完了メッセージを標準出力に表示する。付けない場合、成功時は何も出力しない(ファイル読み込み失敗・パースエラー・型チェック失敗などのエラーは`-v`/`--verbose`の有無に関わらず常に出力される)。`-i`/`--import <名前>=<importパス>`は繰り返し指定でき、指定した名前・パスの組を明示的な`import`として生成コードに先出しで追加する(実際に使われなければgoimportsが自動的に取り除く)。短縮形・長形式はどのオプションも完全に同じ意味を持つ。

- 意味の正しさ(型整合性、未定義識別子、メソッド存在チェックなど)は`go/types`に全面的に委ね、AMIVM側で独自の検証ロジックは持たない
- IR行番号と生成Goコードのエラー行を対応付ける仕組みは、当面の実装では省略する。ただし4節の命名規則により、エラーメッセージから「どの関数由来か」は分かるようになっている

## 16. 過去の設計判断からの変更点(経緯メモ)

### CLIから`go build`を切り離す変更

以前は`go run main.go <IRファイルパス>`という呼び出し前提で、内部で`go build -o output ...`まで実行し実行ファイルを生成していた。しかし当初のパイプライン図(`amivm_spec.md`/`CLAUDE.md`)が示す通り「AMIVM → Goコード」と「Goコード → 実行ファイル」は本来別工程であり、amivmコマンド自身が実行ファイル生成まで担う必然性は無い。そこで`amivm <IRファイルパス> [-o <出力ファイルパス>] [-v]`というコマンド仕様に変更し、`go build`の実行そのものを削除した。出力ファイルパスを明示的に選べるようにする`-o`、および進捗表示を選べる`-v`もこのタイミングで導入した。

### 型チェックをgo/packagesベースに変更し、`-i`/`--import`オプションを追加した経緯

内部の型チェックには当初`go/types`の`types.Config{Importer: importer.Default()}`を直接使っていた。これは標準ライブラリ(GOROOT配下)しか解決できないGOPATH時代の仕組みで、生成したコードが呼び出し側言語実装(xxlang等)が用意する独自のGoライブラリのような標準ライブラリ以外のパッケージを参照すると、実際には`go build`で正しくビルドできるコードであっても、amivm内部の型チェックだけが「could not import」で失敗していた。加えて、型チェックには生成した1ファイルだけを単独で渡していたため、出力先と同じディレクトリ・同じpackageに置かれた別のGoファイル(手書きのランタイムコード等)で定義された識別子も常に「undefined」になっていた。

これを`golang.org/x/tools/go/packages`(`go list`を使う、モジュールを正しく理解するパッケージローダー)ベースの`typeCheck`関数に置き換えることで、標準ライブラリ以外のパッケージへの参照(同一package内の他ファイル・同一モジュール内の別package・モジュールの依存として正しく導入された別モジュールのパッケージ、いずれも)を解決できるようにした。ただしこの仕組みが機能するのは出力先ディレクトリがGoモジュール(`go.mod`が存在する)である場合に限られる(`go build`自身がそうであるように、モジュール外では単一ファイルだけの`command-line-arguments`パッケージとして扱われるため)。

さらに、`imports.Process`(goimports)が`?xxrt.Helper`のような裸の識別子から正しいimportパスを自動推測する部分は、上記の型チェックの改善とは独立した別の問題として残っていることが分かった。標準ライブラリや既に参照済みのパッケージは高確率で解決できるが、まだどこからも参照されていない新規パッケージに対しては、importの挿入自体に失敗する、あるいは誤ったパスを挿入することがある(実在の`github.com/google/uuid`パッケージでも、裸の`uuid.New()`という参照だけからは`import "uuid"`という誤ったパスが挿入される事例を確認した)。この推測の不確実性を完全に回避するため、`-i`/`--import <名前>=<importパス>`オプションを追加した。指定したマッピングは常にエイリアス付きの明示的な`import`文として生成コードの先頭に追加してからgoimportsに渡すため、goimportsは「既にある正しいimportを保つ・使われていなければ消す」という通常の未使用import除去の仕組みで処理するだけになり、推測が一切不要になる。

### 実際に言語実装を書いてみて見つかった4つの仕様不備の修正

amivm-IRを使って2つの言語実装を書いてみたところ、次の4つの不備が見つかった(回避策はあったが、いずれも本来amivm側で解消すべきもの)。

1. **`ADDR`が単純な`&variable`しか表現できなかった**。`&p.x`(構造体フィールドのアドレス)や`&xs[0]`(スライス/配列要素のアドレス)を、既存命令の組み合わせでは取得できなかった。`ADDR single1 variable (point)`という形に拡張し、`point`の有無・種類(`>xxx`かそれ以外か)で`single1 = &variable` / `&variable.point` / `&variable[point]`に分岐するようにした。`point`という新カテゴリは`whole`(0以上の整数)に`>xxx`(フィールド名)を加えただけで、新しいKindの追加は不要だった
2. **`callname`にクロージャー引数(`&N`)・パラメータ(`$N`)が含まれていなかった**。パラメータとしてクロージャーを受け取った関数の中で、そのクロージャーをそのまま呼び出せない(`CALL : $1 ...`ができない)という、完全な仕様ミスだった。`callname`に`$N`/`&N`を追加して解消した
3. **mapを走査する手段が無かった**。AMIVM-IRには`for range`に相当する命令が無く、mapの全キーを列挙する方法が用意されていなかった。`MPKEYS single1 single2`(`single1 = slices.Collect(maps.Keys(single2))`)を新設して解消した
4. **`value`に「関数そのもの」を渡す手段が無かった**。関数を呼び出す(`CALL`)ことはできても、関数を値として(呼び出さずに)変数や引数に渡すことができなかった。`value1 value2`カテゴリに`!xxx`/`?xxx`/`?xxx.yyy`を追加して解消した

いずれも、既存のKind体系・命令セットの枠組みの中で「カテゴリの許容範囲を広げる」「命令に省略可能な引数を1つ足す」「命令を1つ足す」という形で解決でき、Kindの新設や既存命令の破壊的変更は不要だった。

### `callname`への`@xxx`追加、`CLOS`の代入先を`local`から`shallow`に拡張した経緯

上記と同様、実装を進める中で見つかった不備。`callname`(呼び出し対象)は`%xxx`(ローカル変数に保持した関数値・メソッド値・クロージャー)を呼び出せるのに、同じ役割をパッケージレベル変数(`@xxx`)で持たせることができなかった。また`CLOS`の代入先は`local`(`%xxx`のみ)に固定されており、関数パラメータ(`$N`)やパッケージレベル変数(`@xxx`)にクロージャーを代入することができなかった。どちらも「クロージャー・関数値を保持できる変数の種類が、ローカル変数(`%xxx`)に偏っている」という同根の制約で、パッケージレベルで関数値を保持したい(例: シングルトン的なコールバックテーブル)ケースや、パラメータで受け取ったクロージャーをその場で差し替えたいケースに対応できなかった。

解消のため、`CLOS`の代入先カテゴリを`local`(`%xxx`のみ)から新設の`shallow`(`$N`/`%xxx`/`@xxx`。関数レベル以上の変数)に拡張し、`callname`に`@xxx`を追加した。`shallow`が`&N`(クロージャー引数)を含まないのは、`&N`はクロージャー本体(関数リテラル)ごとに閉じたスコープしか持たず、外側で`CLOS`の代入先として参照する対象にならないため。

この変更にあわせて、`single`カテゴリの表記も「命令内の1つ目のオペランドは`single1`、2つ目は`single2`」というルールに統一した(元々`MPKEYS`だけがこの命名だったが、他の命令は単に`single`のままで表記が揺れていたため)。

### `CLOS`をネスト可能にし、`shallow`カテゴリを`single1`に統合した経緯(上記`shallow`新設の直後の変更)

上記で`shallow`を新設した直後、`CLOS`自体をネストできるようにしたい(クロージャーを返すクロージャー、カリー化のようなパターンを表現したい)という要望が出た。`CLOS`は当時`FUNC`/`STTYPE`/`SEL`と同様にネスト禁止だったため、まずこの制約を`CLOS`だけ緩めた。

ネストを許すと、`shallow`が`&N`(クロージャー引数)を除外していた理由が消える。内側の`CLOS`から見れば、外側の`CLOS`のクロージャー引数(`&N`)も「既に存在する、代入し直せる変数」という点で`$N`(外側`FUNC`のパラメータ)と全く同じ扱いをして構わない(実際`$N`は元々`shallow`に含まれていた)。そのため`shallow`という専用カテゴリを廃止し、`CLOS`の代入先には元々`&N`を含んでいた`single1`をそのまま流用することにした。

ネストを許すことで新たに必要になったのが、クロージャー引数の階層識別。ネスト前は`CLOS`が常に1つしか存在しなかったため、クロージャー引数を`amivm_closure_paramN`という関数名修飾なしの固定名にしても衝突しなかった。ネストすると、内側の`CLOS`が外側と同じパラメータ番号(例えば両方とも1番目のパラメータ)を使った場合、Goの通常のスコープルールで外側がシャドーイングされ、外側のパラメータを内側から参照する手段が失われる。これを避けるため、`&N`を「自分がいる`CLOS`階層のN番目」、`&L-N`を「階層`L`のN番目を明示指定」と再定義し、Go側の実名も`amivm_closureL_paramN`とした。`&N`(階層省略形)を「常に最も外側(階層1)」ではなく「自分がいる階層」の意味にしたのは、内側の`CLOS`本体で最も書く頻度が高いのは(外側ではなく)自分自身のパラメータへの参照であり、短い記法がその頻出ケースを指すようにする方が直感に合うため。ネスト前の非ネストIR(常に1階層しかない)との後方互換性は、どちらの意味を選んでも(階層1しか存在しない以上)自動的に保たれるため、この選択に後方互換上の制約はなかった。

### 文字列・ルーンリテラルのエスケープシーケンス対応

`reStringLit`/`reRuneLit`(旧実装)は、それぞれ「`"`で囲まれた`"`以外の文字の並び」「`'`で囲まれた任意の1文字」という簡略化された正規表現で、実際にはGoの正規のエスケープシーケンスの多くを弾いてしまっていた。特に文字列リテラル内の`\"`(エスケープされたダブルクォート)は、`"`という文字そのものが正規表現の許可集合から除外されていたため、含まれているだけで文字列全体がKInvalid判定になっていた。ルーンリテラルは「任意の1文字」しか許さない実装だったため、`'\n'`のような2文字以上からなるエスケープシーケンスや`'\U0001F600'`のようなUnicodeコードポイント指定は、正当なGoのルーンリテラルであるにもかかわらず全て弾かれていた(唯一、非ASCIIの1文字("あ"等)は、Goの`regexp`がデフォルトでルーン単位にマッチするため元から通っていた)。

修正後は、文字列は「エスケープされていない`"`以外の文字」または「`\`に続く任意の1文字」の繰り返しとして再定義し、`\"`を含む任意のエスケープシーケンスを1つの文字列トークンとして認識できるようにした。ルーンは、Unicode 1文字、Goの名前付きエスケープ(`\a`/`\b`/`\f`/`\n`/`\r`/`\t`/`\v`/`\\`/`\'`/`\"`)、`\uXXXX`(4桁hex)、`\UXXXXXXXX`(8桁hex)のいずれかを許容するよう拡張した。`\xHH`(2桁hexバイト値)・8進数バイト値エスケープ(`\nnn`)は、`\u`/`\U`で任意のUnicodeコードポイントを直接指定できるため意図的に対象外とした(整数・浮動小数点・虚数リテラルについても、Goのリテラル構文を完全に模倣する必要性は薄いと判断し、現状の単純な形式のまま維持している)。個々のエスケープが実際にGoとして妥当かどうか(存在しないコードポイント等)まではAMIVM側で検証せず、`go/types`による再パースに委ねる(9節参照)。

### `IF`/`LOOP`をブロック構造にし、`SEL`のケース本体をブロック化し、`ASSERT`を追加した経緯

amivm-IRを使って言語実装を書いてみたところ、次の2つの不備が見つかった。

1. **`any`型の変数にネイティブ命令(算術演算・比較演算等)が使えない**。`ADD`/`EQ`のようなネイティブ命令は、オペランドが具体的な型を持つことを前提にしている。Goのインタフェース型`any`に格納された値をそのままこれらの命令に渡すと、`go/types`が型エラーを返す。`any`から具体的な型の値を取り出す手段(Goの型アサーション`v.(T)`)がAMIVM-IRに存在しなかったため、`any`型の変数を宣言できても実質的に使い物にならなかった。
2. **`goto`が変数宣言を飛び越えてはいけない**。Go言語仕様上、`goto`文は「ジャンプ先の時点で、ジャンプ元では見えていなかった変数がスコープに入る」ことを許さない。旧仕様の制御フロー(単一行`IF boolean1 label`と`LABEL`/`GOTO`だけで`if`/`else`・ループを組み立てる、いわば「構造化されていないgoto方式」)は、条件分岐やループの本体に相当する範囲を、ブロックに区切らず同じ関数スコープの中に並べた`goto`の連鎖として表現していた。そのため、その範囲内で`VAR`宣言をすると、それより後ろにある`goto`(ループの先頭に戻るための`goto`など)がその宣言を飛び越えてしまい、`go/types`が「goto jumps over declaration」相当のエラーを返す事例が実際に発生した。

1は`ASSERT`(型アサーション)を新設して解消した。Goの型アサーション`v.(T)`はキャスト`T(v)`と構文が紛らわしいものの、`ast.CallExpr`ではなく`ast.TypeAssertExpr`という別のASTノードになるため、既存の「キャスト・組み込み関数は`CALL`に統合する」方針(9節参照)にそのまま乗せることができず、専用命令として追加した。

2は、単一行`IF`を廃止して`ELIF`/`ELSE`/`ENDIF`を伴うブロック形の`IF`に置き換え、新たに`LOOP`/`BREAK`/`CONTINUE`/`ENDLOOP`という無限ループ構造を追加して解消した(11節・12節参照)。Goの`if`/`for`はそれぞれ独立したブロックスコープを持つため、本体内で`VAR`宣言をしても、その宣言はブロックを抜けた時点でスコープごと消える。ループ内で「宣言→(条件によって)手前に戻る」という処理を書いても、戻り先(ブロックの先頭)は宣言より前なので、この種の飛び越えが構造的に発生しない。`LABEL`/`GOTO`自体は、構造化制御フローだけでは書けないジャンプ(Goの`goto`が元々担っている領域)のために残してあり、廃止していない。

`IF`/`LOOP`を導入すると、それまで`FUNC`/`STTYPE`/`SEL`と同様にネスト禁止だった制約のうち、`SEL`だけ据え置くのは一貫性を欠く。`IF`の中に`LOOP`、その中に`IF`、というネストは構造化制御フローとして当然必要になるため、`IF`/`LOOP`/`CLOS`/`SEL`をまとめて「ネスト可能」に統一した(`CLOS`は既にネスト可能だったので変更なし)。この際、`SEL`の`CASESEND`/`CASERECV`/`DEFAULT`が「1行で完結し`label`へ`goto`する」という旧仕様のままだと、`SEL`のケースの中に`IF`/`LOOP`を書く余地が無く、「`SEL`もネストできる」という説明と実態が食い違う。そこで`CASESEND`/`CASERECV`/`DEFAULT`から`label`を撤去し、Goの`select`の`case`/`default`節そのままに、次のケースか`ENDSEL`までを本体として持つブロックに作り替えた(3節参照)。

実装面では、`IF`の各分岐(`IF`/`ELIF`本体・`ELSE`本体)も`SEL`の各ケース本体も、突き詰めると「複数の候補キーワードのどれかが現れるまで文リストを読む」という同じ形をしている(`ELIF`/`ELSE`/`ENDIF`のどれか、または`CASESEND`/`CASERECV`/`DEFAULT`/`ENDSEL`のどれか)。そのため`parseBody`の第5引数を単一の終端キーワード(`string`)から終端キーワードの集合(`[]string`)に一般化し、どのキーワードで止まったかも合わせて返すようにした。これにより`IF`・`SEL`のどちらも同じ`parseBody`を使い回しつつ、`ELSE`の後に`ELIF`が続くような不正な並びは、`parseBody`が「今読んでいる本体の終端候補に含まれない予約語(`ELIF`/`ELSE`/`ENDIF`/`ENDLOOP`)」を検出した時点で構文エラーとして弾く(`amivm_spec.md`4.10節参照)、という一貫した仕組みで実現できた。

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
- **旧`ch`カテゴリ(チャネル/スライス共有)を`single1`/`single2`(当時は`single`)に統合**し、意味を「単一左辺、チャネル/スライス/マップ/構造体変数」に整理した
- **`CLOS`/`ENDCLOS`(クロージャー)を追加**。`&N`という専用プレフィックスの引数を持つ

### それ以前の変更点

- **チャネルを宣言不可としていた制約を撤廃**。`:=`を使うことによる変数シャドーイングのバグをきっかけに、「全ての変数は宣言→=で代入」という一貫したルールに統一する方が構造的に安全だと判断した
- **トークンの分類(Kind)に、値だけでなく型の形状も含めるよう統一**
- **`CONST`/`CONV`/`PRINT`を削除**。`CONST`はフロントエンド側のリテラル埋め込みで代替、`CONV`はGoの型変換`T(v)`が`ast.CallExpr`と同一構造のため`CALL`で表現可能、`PRINT`は動作確認用の暫定命令で`CALL`(`?fmt.Println`)により代替可能と判断した
- **スライスを導入**。配列は型に長さが埋め込まれ関数の引数・戻り値に不向きなため
- **全識別子にプレフィックス記号を導入**。以前`@`(AMIVM関数)/`?`(Go関数)にのみ導入していた「先頭記号で種別を明示する」考え方を、変数・型・ラベル・フィールドまで含めた全カテゴリに拡張した
- **`VAR`(関数内)と`GVAR`(関数外)を明確に別命令として分離**
- **未使用変数の`_ = x`挿入方式を、AST全体検索から命名規則ベースの文字列分割に変更**。挿入位置もVAR宣言の直後に変更し、`missing return`エラーの誘発を防いだ
