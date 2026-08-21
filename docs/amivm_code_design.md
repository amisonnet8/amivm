# AMIVM お試し実装 コード設計メモ

コンパイラ本体(`cmd/amivm/`配下の単一パッケージ`package main`)の内部構造を、処理の流れに沿って説明する。命令セットそのものの仕様は`amivm_instruction_spec.md`(および唯一の正確な仕様である`amivm_spec.md`)を参照。

実装は処理の層ごとに、`cmd/amivm/`配下の以下のファイルへ分割されている。各節がどのファイルに対応するかを併記する。

| ファイル | 対応する節 |
|---|---|
| `token.go` | 1節(トークナイズ+分類) |
| `parse_stmt.go` | 2節(命令の判定・1行完結命令のパース)、4節(`splitColon`) |
| `astbuild.go` | 3節の`atomToExpr`・命名規則ヘルパー |
| `category.go` | 3節の`Category`/`allowedKinds`/`atomExpr`/`checkKind` |
| `parse_block.go` | 5節(ブロック構造の組み立て、`TYPE`系宣言のパース) |
| `program.go` | 5節の`buildProgram`(トップレベルの組み立て) |
| `compile.go` | 6節(未使用変数の救済)、7節(Goソース出力パイプライン) |
| `main.go` | 8節(エントリポイントとCLI引数の解釈) |

## 全体像

```
IRテキスト
  ↓ 1. 行の前処理(空行・//コメント行の除去、インデント除去)
  ↓ 2. トークナイズ+分類(タブ区切り分割 → 各トークンをその場でclassify)
[]Atom(行ごと、既にKindが確定した状態)
  ↓ 3. 命令の判定(先頭Atomの文字列で分岐)
  ↓ 4. 各命令のパース(Atom列を読み、カテゴリと照合するだけ)
ast.Expr / ast.Stmt
  ↓ 5. ブロック構造の組み立て(FUNC/SEL/CLOS/STTYPE)
ast.File
  ↓ 6. Goソース出力パイプライン(format → import解決 → 型チェック・未使用変数の自己修復 → 書き出し)
Goソースファイル
```

Goソースファイルを実行ファイルにする`go build`は、amivmコマンド自身は行わない別工程として切り離されている(7節参照)。

設計の要点は、**「トークンが何者か(Kind)」を判定する処理と、「命令が何か」を判定する処理を独立させ、前者を後者より先に行う**こと。行を読んだ時点でトークン列は全て分類済みの`Atom`になっており、以降の命令別パース処理は一切`classify`を呼ばず、既に確定した`.Kind`をカテゴリ(許可リスト)と照合するだけで済む。

## 1. トークナイズ+分類

`tokenizeAndClassify(line string) []Atom`

行をタブ文字で分割し、**その場で各トークンをclassifyにかけて`Atom`(種別+補助情報)にする**。命令が何であるかはまだ分からない段階だが、個々のトークンの「形」(整数リテラルか、識別子か、`$N`か、配列型かポインタ型か等)はこの時点で全て確定する。

```go
func tokenizeAndClassify(line string) []Atom {
    var atoms []Atom
    for _, f := range strings.Split(line, "\t") {
        f = strings.TrimSpace(f)
        if f != "" {
            atoms = append(atoms, classify(f))
        }
    }
    return atoms
}
```

区切り文字にタブを選んだ理由は、文字列リテラル(`"hello world"`)がスペースを含んでいても、タブで分割する限り1トークンのまま保てるため。

`splitLinesTrimmed`が行の前処理を担う。各行の前後の空白(タブによるインデント含む)を取り除いた上で、空行と`//`で始まるコメント行を除外する。これにより後段の処理は「意味のある行」だけを見ればよくなる。

```go
func splitLinesTrimmed(source string) []string {
    var lines []string
    start := 0
    for i, c := range source {
        if c == '\n' {
            line := strings.TrimSpace(source[start:i])
            if line != "" && !strings.HasPrefix(line, "//") {
                lines = append(lines, line)
            }
            start = i + 1
        }
    }
    // ...末尾行も同様...
    return lines
}
```

なお、ブロック構造の判定(`FUNC`/`SEL`/`CLOS`/`STTYPE`/`ENDxxx`等の予約語チェック)や`FUNC`シグネチャ・`LABEL`名の解析も、この`tokenizeAndClassify`をそのまま利用する。値の分類と型の分類を1つの`Kind`体系に統一しているため、ブロック構造の解析でも同じ`classify`を使い回せる。

## 2. 命令の判定(第1段階: 緩い判定)

`parseSingleLine(line, funcName string) (ast.Stmt, error)`

`tokenizeAndClassify`で分類済みの`Atom`列を受け取り、**先頭Atomの文字列(`Raw`)だけ**を見て、対応するパース関数に振り分ける。命令名(`VAR`, `ADD`など)は予約語なので、`Kind`(値としての種別)ではなく`Raw`文字列で判定する。

```go
func parseSingleLine(line, funcName string) (ast.Stmt, error) {
    atoms := tokenizeAndClassify(line)
    kw := atoms[0].Raw
    rest := atoms[1:]
    switch kw {
    case "VAR":
        return parseVar(rest, funcName)
    case "ASET":
        return parseAset(rest, funcName)
    case "FSET":
        return parseFset(rest, funcName)
    case "MSET":
        return parseMset(rest, funcName)
    // ...
    default:
        return nil, fmt.Errorf("未知の命令です: %s", line)
    }
}
```

ここで扱うのは関数本体内で1行完結する命令(`VAR`/`SET`/`ASET`/`AGET`/`PSET`/`PGET`/`ADDR`/四則演算・ビット演算・シフト・論理・比較演算/`LABEL`/`GOTO`/`IF`/`RET`/`CALL`/`DEFER`/`SPAWN`/`CHMAKE`/`SLMAKE`/`MPMAKE`/`CHSEND`/`CHRECV`/`CONCAT`/`SLICE`/`FSET`/`FGET`/`MSET`/`MGET`/`MPKEYS`)。`LABEL`は`label: ;`という固定の形を生成するだけの1行命令であり、他の1行命令と全く同じにここで扱える(先読みは不要。5節参照)。複数行にまたがる`FUNC`/`SEL`/`CLOS`/`STTYPE`はここでは扱わず、5節の`parseBody`/`buildProgram`側で処理する。`GVAR`・`CHTYPE`/`SLTYPE`/`MPTYPE`/`FNTYPE`もトップレベル専用のため、`buildProgram`でのみ処理する。

## 3. トークンの分類体系(Kind)

### 識別子側は複合形を持たない

以前の設計では識別子(`$N`/`%xxx`/`@xxx`)に添字(`[i]`)・スライス(`[f:t]`)・デリファレンス(`*`)・アドレス取得(`&`)を埋め込んだ複合形をKindとして持っていたが、現行仕様ではこれらは全て`ASET`/`AGET`/`PSET`/`PGET`/`ADDR`/`SLICE`という専用命令に分離されている。結果として識別子側のKindは`$N`(`KParam`)・`&N`(`KClosureParam`)・`%xxx`(`KLocal`)・`@xxx`(`KGlobal`)・`@xxx.yyy`(`KGlobalSel`)の5種類だけになり、`classify`の実装・`Category`の許可リストの両方が大幅に単純化された。

```go
type Kind int

const (
    KInvalid Kind = iota
    KBool
    KZero
    KPosInt
    KNegInt
    KFloat
    KRune
    KString
    KNil
    KBlank
    KColon

    // $ / & / % / @ (識別子。複合形なし)
    KParam        // $N
    KClosureParam // &N
    KLocal        // %xxx
    KGlobal       // @xxx
    KGlobalSel    // @xxx.yyy

    // > (構造体フィールド名)
    KField

    // ^ (型。単純/セレクタ/ポインタ/配列の4系統×セレクタ有無)
    KType
    KTypeSel
    KTypePtr
    KTypePtrSel
    KArrType
    KArrTypeSel
    KArrTypePtr
    KArrTypePtrSel

    // ! ? (関数)
    KAmivmFunc
    KAmivmMain
    KGoFunc
    KGoFuncSel

    // # (ラベル)
    KLabel
)
```

`KInvalid`を含めて30種類(識別子系は5・型系は8・関数系は4・ラベル1・フィールド1・リテラル/記号系11)。以前(33種類、うち多くが配列/スライス/チャネル型のインライン形やパラメータの複合形だった)から純減している。理由は、コンテナ型(チャネル/スライス/マップ/構造体/関数型)のインライン埋め込みが廃止され`TYPE`系命令による`deftype`参照に一本化されたため(型側のKindも配列型4パターンのみに削減)、および識別子の複合形が専用命令に分離されたため。

`Atom`は`A`, `B`, `C`という3つの汎用フィールドを持ち、Kindごとに意味が変わる(例: `KArrTypeSel`なら`A`=サイズ, `B`=要素の左側, `C`=要素の右側)。フィールド名を都度専用のものに増やすのではなく、コメントで意味を明記する形にして構造体自体は小さく保っている。

### `classify` — 判定順序

記号なしのリテラル(`true`/`false`/`nil`/`_`/`:`/数値/文字/文字列)を先に判定し、残った場合は先頭1文字(`$`/`&`/`%`/`@`/`^`/`!`/`?`/`#`/`>`)で名前空間を判定して専用のサブパーサーに委譲する。以前存在した「先頭の`*`/`&`(デリファレンス・アドレス取得)を剥がしてから記号を見る」という前処理は、複合形の廃止に伴い不要になった。

```go
func classify(tok string) Atom {
    switch {
    case tok == "true" || tok == "false":
        return Atom{Kind: KBool, Raw: tok}
    // ...true/false/nil/_/: の判定...
    }
    // ...数値・rune・文字列リテラルの判定...
    switch tok[0] {
    case '$':
        return classifyParam(tok, tok[1:])
    case '&':
        return classifyClosureParam(tok, tok[1:])
    case '%':
        return classifyLocal(tok, tok[1:])
    case '@':
        return classifyGlobal(tok, tok[1:])
    case '^':
        return classifyType(tok, tok[1:])
    case '!':
        return classifyAmivmFunc(tok, tok[1:])
    case '?':
        return classifyGoFunc(tok, tok[1:])
    case '#':
        return classifyLabel(tok, tok[1:])
    case '>':
        return classifyField(tok, tok[1:])
    default:
        return Atom{Kind: KInvalid, Raw: tok}
    }
}
```

`0`と正の整数・負の整数を別Kind(`KZero`/`KPosInt`/`KNegInt`)に分けているのは、`whole`(0以上の整数。添字・シフト量・サイズ等)カテゴリが「0以上」しか許さないのに対し、`integer`カテゴリは負数も許す、という区別を`Category`の許可リストだけで表現するため(`0`と負数をまとめて1つのKindにすると、この区別を別ロジックで行う必要が出てくる)。

### `atomToExpr` — AST組み立て

```go
func atomToExpr(a Atom, funcName string) (ast.Expr, error) {
    switch a.Kind {
    case KParam:
        return paramBaseExpr(funcName, a.A)
    case KClosureParam:
        return closureParamBaseExpr(a.A), nil
    case KArrType:
        return arrayTypeExpr(a.A, ast.NewIdent(a.B))
    case KArrTypeSel:
        return arrayTypeExpr(a.A, &ast.SelectorExpr{X: ast.NewIdent(a.B), Sel: ast.NewIdent(a.C)})
    // ...
    }
}
```

`$N`→Go識別子(`paramBaseExpr`)、`&N`→Go識別子(`closureParamBaseExpr`、関数名による修飾なし)、`%xxx`→Go識別子(`localBaseExpr`)、`@xxx`/`@xxx.yyy`→Go識別子(`globalBaseExpr`/`globalSelBaseExpr`)、配列型組み立て(`arrayTypeExpr`)、スライス型組み立て(`sliceTypeExpr`、`TYPE`系命令からのみ呼ばれる)、チャネル型組み立て(`chanTypeExpr`、同様)という7つのヘルパーに要素部分の`ast.Expr`を渡すだけで済む形にしている。

`ASET`/`AGET`/`PSET`/`PGET`/`ADDR`/`SLICE`/`FSET`/`FGET`/`MSET`/`MGET`が組み立てる`ast.IndexExpr`/`ast.StarExpr`/`ast.UnaryExpr`/`ast.SelectorExpr`/`ast.SliceExpr`は、`atomToExpr`のKind分岐ではなく各命令のパース関数(`parseAset`等)内で直接組み立てる。これらは「ある1つのAtomの形」ではなく「2〜3個のAtomの組み合わせ」から作る式なので、Kind単位で完結する`atomToExpr`には乗らない。

### `Category`と`allowedKinds` — 文脈ごとの許可判定

`amivm_instruction_spec.md`の「オペランドカテゴリ」表を、そのまま`Category`とその許可リストとしてコード化したもの。段階的な包含関係を持つカテゴリは、`mergeKinds`で前段階の許可リストを合成してから追加していくことで、重複した列挙を避けている。

```go
type Category int

const (
    CatVa Category = iota
    CatGv
    CatSingle
    CatMulti
    CatVariable
    CatType
    CatDeftype
    CatField
    CatPoint
    CatWhole
    CatFromTo
    CatInt
    CatNumber
    CatBool
    CatSlice
    CatOrder
    CatValue
    CatDefname
    CatCallname
    CatLabel
)
```

```go
whole := mergeKinds(identRefFull, kindSet(KZero, KPosInt, KRune))
m[CatWhole] = whole
m[CatFromTo] = mergeKinds(whole, kindSet(KBlank))

integer := mergeKinds(whole, kindSet(KNegInt))
m[CatInt] = integer

number := mergeKinds(integer, kindSet(KFloat))
m[CatNumber] = number
```

`identRefFull`(`$N`/`&N`/`%xxx`/`@xxx`/`@xxx.yyy`)と`identRefNoSel`(`@xxx.yyy`を除いた4種)という2つの基礎集合を用意し、ほとんどのカテゴリはどちらかをベースにリテラル系Kindを足すだけで構築できる。`single`/`multi`は`identRefNoSel`ベース(`multi`はさらに`KBlank`を追加)、それ以外の値系カテゴリ(`whole`/`integer`/`number`/`boolean`/`slice`/`ordered`/`value`/`variable`)は`identRefFull`ベース。

`atomExpr(a Atom, funcName string, cat Category) (ast.Expr, error)`が「カテゴリ確認 → `atomToExpr`呼び出し」を直列に実行する共通のエントリポイントで、命令別のパース関数はこれだけを呼べばよい。`classify`は`tokenizeAndClassify`でのみ呼ばれ、命令ごとのパース処理からは一切呼ばれない。

### `checkKind` — 検証のみで組み立てを伴わない場合

`VAR`/`GVAR`の変数名、`LABEL`/`GOTO`/`IF`/`CASESEND`/`CASERECV`/`DEFAULT`のラベル、`FUNC`の関数定義名、`TYPE`系命令の`deftype`名など、「Kindがカテゴリに属するかどうかだけ確認したい(組み立てた`ast.Expr`は使わない)」箇所がある。これらを`atomExpr`経由で行うと2つの問題があった。

1. `%xxx`(`KLocal`)の`atomToExpr`は`funcName`が空だとエラーを返す実装になっているため、検証のためだけに`funcName: ""`を渡すと(実際の変数解決とは無関係に)常に失敗する
2. ラベル(`KLabel`)は`atomToExpr`にKind分岐が無く(ラベル名の取得は別途`labelGoName`が担う)、`default`節に落ちて常に失敗する

どちらも「動かして初めて見つかったバグ」で、実際に`VAR`と`GOTO`が常にエラーになっていた。対処として、Kind判定だけを行い`ast.Expr`を組み立てない`checkKind`を切り出し、検証専用の箇所は全てこちらに置き換えた。

```go
func atomExpr(a Atom, funcName string, cat Category) (ast.Expr, error) {
    if err := checkKind(a, cat); err != nil {
        return nil, err
    }
    return atomToExpr(a, funcName)
}

func checkKind(a Atom, cat Category) error {
    if a.Kind == KInvalid {
        return fmt.Errorf("%sとして解釈できない形式です: %s", categoryLabel[cat], a.Raw)
    }
    if !allowedKinds[cat][a.Kind] {
        return fmt.Errorf("%sにこの形式は使えません: %s", categoryLabel[cat], a.Raw)
    }
    return nil
}
```

### 型・呼び出し対象も同じKind体系に統一されている

`VAR`の型、`FUNC`/`FNTYPE`/`CLOS`の戻り値・パラメータ型、`CHMAKE`/`SLMAKE`/`MPMAKE`の`deftype`はいずれも`atomExpr`(`CatType`/`CatDeftype`)で解決され、値の解決(`CatValue`等)と全く同じ経路を通る。

## 4. `:`(コロン)区切りの解析 — `splitColon`

`CALL`/`FUNC`/`FNTYPE`/`CLOS`は、トークン列の中の`KColon`(`:`)を境に2分割する必要がある。以前は「`!`/`?`で始まる最初のトークンを走査して呼び出し対象を探す」という位置に依存しない方式だったが、現行仕様では`:`という明示的な区切りトークンに統一されたため、次のヘルパーに集約されている。

```go
func splitColon(atoms []Atom) (left, right []Atom, err error) {
    idx := -1
    for i, a := range atoms {
        if a.Kind == KColon {
            if idx >= 0 {
                return nil, nil, fmt.Errorf("コロン(:)は1つだけ指定してください: %s", joinRaw(atoms))
            }
            idx = i
        }
    }
    if idx < 0 {
        return nil, nil, fmt.Errorf("コロン(:)が見つかりません: %s", joinRaw(atoms))
    }
    return atoms[:idx], atoms[idx+1:], nil
}
```

`:`が0個・2個以上のどちらもエラーにする。`FUNC`/`FNTYPE`はこれで「パラメータ型列 / 戻り値型列」を、`CALL`は「代入先(`multi`列) / 呼び出し対象+引数列」を、`CLOS`は「パラメータ型列 / 戻り値型列」(`local`はその前で別途取り出す)を分割する。`DEFER`/`SPAWN`は`:`を使わないため`splitColon`は呼ばない。

## 5. ブロック構造の組み立て

行単位のパースだけでは表現できない、複数行にまたがる構造を扱う部分。

### `parseBody` — `FUNC`/`CLOS`本体の構築

開始位置から、指定した終端キーワード(`ENDFUNC`または`ENDCLOS`)が現れるまでの行を走査し、`[]ast.Stmt`を組み立てる。`FUNC`本体と`CLOS`本体は同じ`parseBody`を再帰的に使い回している(`CLOS`は`funcName`を外側の`FUNC`のものそのまま引き継ぐ。`CLOS`自体は無名で専用の名前空間を持たないため、内部で`VAR`宣言した変数は外側の関数名で修飾される)。

- 通常の行は`parseSingleLine`でそのままパースして追加
- `SEL`行が出てきたら`parseSelectBlock`に処理を委譲する
- `CLOS`行が出てきたら`parseClosSignature`でシグネチャを解析し、`parseBody`を`"ENDCLOS"`終端で再帰呼び出しして本体を構築、`*ast.FuncLit`にラップして`local`への代入文(`ast.AssignStmt`, `token.ASSIGN`)として積む
- それ以外の行(`LABEL`含む)は全て`parseSingleLine`にそのまま渡す

`LABEL`は`amivm_spec.md`の定義(`LABEL label → label: ;`)どおり、**次の行が何であるかに関わらず常に**`&ast.LabeledStmt{Stmt: &ast.EmptyStmt{}}`を生成する1行完結の命令であり、`parseSingleLine`側の通常の`case "LABEL"`(`parseLabel`)で処理する。

```go
// parseSingleLine内
case "LABEL":
    return parseLabel(rest)

// parseLabel
func parseLabel(atoms []Atom) (ast.Stmt, error) {
    // ...checkKind(atoms[0], CatLabel) でラベル名を検証...
    name, err := labelGoName(atoms[0])
    // ...
    return &ast.LabeledStmt{Label: ast.NewIdent(name), Stmt: &ast.EmptyStmt{}}, nil
}
```

これにより`parseBody`側に`LABEL`専用の分岐は存在しない。以前は「`LABEL`の直後の行を先読みし、`SEL`/`CLOS`(ブロック開始行)かどうかで挙動を変える」という特殊なロジックを持っていたが、`amivm_spec.md`の仕様を「ラベルは常に空文とセットで1行完結する」に変更したことで、この先読み処理自体が丸ごと不要になった。`LABEL`の直後にどんな行(通常の1行・`SEL`・`CLOS`・ブロック終端・別の`LABEL`)が来ても、`LABEL`自身の生成結果は変わらず、後続の行は`parseBody`のループが独立した次の1文として扱う。生成される Go コードは次のようになる。

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

`;`(空文)は`SEL`/`CLOS`の前に限らず、全ての`LABEL`の直後に一律で入る。`goto afterSend`で戻ってきたときは空文を経由してそのまま次の文に入るため、意味的には問題ない。

### `parseSelectBlock` — `SEL`本体の構築

`SEL`の次の行から`ENDSEL`までを走査し、`CASESEND`/`CASERECV`/`DEFAULT`行をそれぞれ`ast.CommClause`に変換して`ast.SelectStmt`を組み立てる。各行は1行で完結する(内部にさらにブロックを持たない)ため、`parseBody`より単純な走査で済む。

### `parseStructBlock` — `STTYPE`本体の構築

`STTYPE`の次の行から`ENDSTTYPE`までを走査し、`FIELD field type1`行を`*ast.Field`に変換して`*ast.StructType`を組み立てる。`FIELD`以外の行が現れたらエラーにする(`STTYPE`はトップレベル専用で`parseBody`とは別の走査関数)。

### `buildProgram` — トップレベルの組み立て

トップレベルには`GVAR`・`FUNC`・`CHTYPE`・`SLTYPE`・`STTYPE`・`MPTYPE`・`FNTYPE`だけが並ぶ前提。`FUNC`行を見つけるたびに`parseFuncSignature`でシグネチャを解析し、`parseBody`で本体を構築、`ast.FuncDecl`として`ast.File.Decls`に積んでいく。`CHTYPE`/`SLTYPE`/`MPTYPE`/`FNTYPE`は1行で完結する`TYPE`宣言なので、それぞれ専用のパース関数(`parseChType`等)が直接`ast.Decl`(`typeDecl`ヘルパーで`type deftype ...`の形に組み立てる)を返す。`STTYPE`のみ複数行ブロックのため`parseStructBlock`に処理を委譲する。

`parseFuncSignature`は、`FUNC`直後のトークンを関数定義名(`!xxx`/`!main`)として取り出し、残りを`splitColon`でパラメータ型列と戻り値型列に分割する。パラメータには`amivmParamGoName(defName, i+1)`で名前を振り、戻り値は無名の`*ast.Field`にする(`typeFieldsNamed`/`typeFieldsUnnamed`)。

## 6. 未使用変数の救済 — ネストしたブロックへの対応

`_ = x`の挿入先探索(`insertBlankAfterDecl`)は、対象のVAR宣言文を関数のトップレベル文だけでなく、`IF`本体・`SEL`の各`CommClause`・`CLOS`(`func`リテラル)の内部まで**再帰的に**探索する。

```go
func findAndInsertBlank(list *[]ast.Stmt, varGoName string) bool {
    for i, stmt := range *list {
        if declaresVarDirect(stmt, varGoName) {
            // ...このリストの直後に `_ = varGoName` を挿入して true を返す...
        }
        if insertBlankInNested(stmt, varGoName) {
            return true
        }
    }
    return false
}

func insertBlankInNested(stmt ast.Stmt, varGoName string) bool {
    switch s := stmt.(type) {
    case *ast.BlockStmt:
        return findAndInsertBlank(&s.List, varGoName)
    case *ast.LabeledStmt:
        return insertBlankInNested(s.Stmt, varGoName)
    case *ast.IfStmt:
        // s.Body、s.Else を再帰的に見る
    case *ast.SelectStmt:
        // 各 CommClause.Body を再帰的に見る
    case *ast.AssignStmt:
        // Rhs が *ast.FuncLit(CLOSの本体)なら、その Body.List を再帰的に見る
    }
    return false
}
```

`CLOS`は`func`リテラルとして独立したブロックスコープを作るため、これを省略すると`CLOS`本体で未使用になった`VAR`変数(`fn.Body.List`の直接の要素ではなく、`AssignStmt.Rhs`の`*ast.FuncLit.Body.List`の中にある)を発見できず、命名規則ベースの絞り込み(所属関数の特定)は成功してもVAR宣言そのものが見つからない、という状態に陥る。この場合の安全網として、該当箇所が見つからなければ全関数の末尾に`_ = x`を追加するフォールバックが残っているが、通常は上記の再帰探索で正しい挿入位置が見つかる。

なお、Goの言語仕様上「未使用」でエラーになるのは`var`宣言されたローカル変数のみで、関数パラメータ(`$N`)やクロージャー引数(`&N`)は未使用でもエラーにならないため、この救済処理の対象になるのは常に`VAR`由来の変数だけである。

## 7. Goソース出力パイプライン

`generateOutput`が、組み立てた`ast.File`を実際のGoソースファイルにする。以前は`generateAndBuild`という名前で、末尾に`go build -o output ...`を外部プロセスとして実行し実行ファイルまで生成していたが、現在は削除されている(後述)。

```
ast.File
  → format.Node でテキスト化
  → imports.Process で import解決
  → parser.ParseFile で再パース(構文エラー検出)
  → typeCheck(go/packages経由)で型チェック
      - 未使用変数エラーのみ検出 → 6節の再帰探索で `_ = x` を挿入して再チェック(最大5回ループ)
      - それ以外のエラー → 即座に失敗として返す
  → os.WriteFile で出力先パスへ書き出し
```

型の整合性・未定義識別子・関数シグネチャの不一致・メソッドの存在チェックなどは、AMIVM側で検証せず`go/types`(`typeCheck`関数経由)に委ねている。

### `typeCheck` — go/packagesによるモジュール対応の型チェック

以前は`types.Config{Importer: importer.Default()}`を直接使っていたが、`importer.Default()`はGOROOT配下の標準ライブラリしか解決できず、Goモジュールを一切理解しない(GOPATH時代の仕組みをそのまま使っている)。そのため、生成したコードが標準ライブラリ以外のパッケージ(利用側言語実装が用意する独自のランタイムライブラリ等)を参照すると、実際には`go build`で正しくビルドできるコードであっても、amivm内部の型チェックだけが「could not import」で失敗するという問題があった。加えて、`go/types`の`Check`に生成した1ファイルだけを単独で渡していたため、出力先と同じディレクトリ・同じpackageに置かれた別のGoファイル(手書きのランタイムコード等)で定義された識別子も常に「undefined」になっていた。

`typeCheck`はこれを`golang.org/x/tools/go/packages`(内部で`go list`を使う、Goモジュールを正しく理解するパッケージローダー)に置き換えることで解決している。

```go
func typeCheck(outputPath string, resolved []byte) (unusedVars []string, otherErrs []string, err error) {
    absOutputPath, err := filepath.Abs(outputPath)
    // ...
    cfg := &packages.Config{
        Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo |
            packages.NeedImports | packages.NeedDeps | packages.NeedSyntax,
        Dir:     filepath.Dir(absOutputPath),
        Overlay: map[string][]byte{absOutputPath: resolved},
    }
    pkgs, err := packages.Load(cfg, "file="+absOutputPath)
    // ...
}
```

ポイントは2つ。

1. **`Overlay`でディスクに書き込まずにチェックする**。`resolved`(まだファイルに書き出していない、生成した最新のGoソース)を`absOutputPath`の内容として差し替えるため、実際にファイルを書かなくても「そこにそのファイルがあるとしたら」という前提でパッケージ全体を読み込める。`Overlay`のキーは`go/packages`の仕様上絶対パスである必要があるため、`filepath.Abs`で変換してから渡す(相対パスのままだとファイルが存在しない扱いになり解決に失敗する)
2. **`"file="+absOutputPath`という問い合わせにより、出力先ファイルが属するパッケージ全体(同じディレクトリの他の.goファイルを含む)が読み込まれる**。これにより「同じpackage内の手書きファイルで定義された関数」も「同じモジュール内の別packageの関数」も、`go build`が実際に解決できるのと同じルールで解決できるようになる

ただし、この仕組みが機能するのは**出力先ディレクトリがGoモジュール(`go.mod`が存在する)である場合**に限られる。`go.mod`が無いディレクトリでは、`go build <file>`と同様に単一ファイルだけの`command-line-arguments`パッケージとして扱われ、同じディレクトリの他ファイルは読み込まれない(標準のGoの挙動であり、amivm側で変えられるものではない)。`test_ir/`のテスト(`make test`)は標準ライブラリ呼び出ししか使わないため、この制約の影響を受けず`go.mod`無しの一時ディレクトリでも問題なく動作する。

### importパスの自動推測は信頼できない — `-i`/`--import`オプションによる対処

`typeCheck`が解決するのは、あくまで「**import文が既に存在する**参照」である。`imports.Process`(goimports)が`?xxrt.Helper`のような裸の識別子から`import "xxlangmodule/xxrt"`のようなimport文を自動的に**推測して挿入する**部分は、`typeCheck`の変更の対象外であり、かつ挙動が不安定であることが分かっている。標準ライブラリや、既にどこかから参照されて「馴染みのある」パッケージは高い確率で解決できるが、まだどこからも参照されていない新規のパッケージ(独自ライブラリを導入した直後など)に対しては、importの挿入自体に失敗する(識別子が未解決のまま残る)場合や、誤ったimportパスを挿入してしまう場合がある。これは`golang.org/x/tools/imports`の軽量なAPI(`imports.Process`)自体の制約であり、`typeCheck`側をいくら正確にしても解消しない別問題である。

この不確実性を、goimportsの推測を経由させないことで回避するのが`-i`/`--import <名前>=<importパス>`オプション(8節)。`main.go`の`injectExplicitImports`が、指定されたマッピングを`file.Decls`の先頭に明示的な`*ast.GenDecl{Tok: token.IMPORT}`として(エイリアス付きで)追加してから`generateOutput`(→`compileOnce`→`imports.Process`)に渡す。goimportsは「既にある正しいimportを保つ・実際に使われていなければ消す」という通常の未使用import除去の仕組みで処理するだけなので、推測が一切不要になる。

`generateOutput`は`verbose bool`引数を取り、`true`のときだけ「未使用変数を検出したため`_ = x`を挿入します」というログと「最終生成コード」のダンプを標準出力に書く。`false`のときはこれらを一切出力しない(8節参照)。

### なぜ`go build`(実行ファイル生成)を行わないか

`amivm`コマンドの責務はGoソースファイルを出力するところまでで、実行ファイルの生成は別工程として扱う。これは`amivm_spec.md`/`CLAUDE.md`のパイプライン図(`対象言語 → AMIVM-IR → AMIVM → Goコード → 実行ファイル`)で「AMIVM → Goコード」と「Goコード → 実行ファイル」が別の矢印として描かれていることに対応する。`go/types`による型チェック(上記パイプラインの一部)は既にGoコードとしての正しさを検証しているため、実際に`go build`で実行ファイルまで作らなくても出力の妥当性は担保できる。

## 8. エントリポイント(`main`)とCLI引数の解釈

`main`は次の4つの責務に分かれる。

1. `parseArgs(os.Args[1:])`でコマンドライン引数を解釈する
2. IRファイルを読み込み、`buildProgram`でastを組み立てる
3. `-i`/`--import`が指定されていれば`injectExplicitImports`で明示的なimportを追加してから`generateOutput`のパイプラインに通す
4. `verbose`に応じて標準出力の有無を切り替える。エラーは`verbose`の値に関わらず常に出力する

```
amivm <IRファイルパス> [-o|--output <出力ファイルパス>] [-v|--verbose] [-i|--import <名前>=<importパス>]...
```

`parseArgs`は`os.Args`を1つずつ見て`-o`/`--output`(次のトークンを値として消費)・`-v`/`--verbose`(フラグ)・`-i`/`--import`(次のトークンを`name=path`として消費、繰り返し可)・それ以外(IRファイルパス、1個まで)に振り分ける、順序に依存しない小さな手書きパーサーである。短縮形と長形式は`switch`の同じ`case`に列挙しているだけで、挙動は完全に同じ。標準ライブラリの`flag`パッケージは最初の非フラグ引数でパースを止めてしまう(`amivm input.ir -v`のように位置引数を先に書くと`-v`が拾えない)ため、`amivm <path> -o out.go -v`のような自然な書き方も許容するために独自実装にしている。

`-i`/`--import`の各値は`parseImportArg`で`name`と`path`に分解される。`name`はGoの識別子として妥当な形式(`^[A-Za-z_][A-Za-z0-9_]*$`)であることを検証し、同じ`name`が複数回指定された場合はエラーにする(サイレントな上書きを避けるため)。`injectExplicitImports`はこの`map[string]string`を受け取り、決定的な出力にするためキーをソートしてから`*ast.ImportSpec`(常にエイリアス付き。パッケージの実際の宣言名が`name`と一致しない場合でも安全に動く)の並びを組み立て、`file.Decls`の先頭に追加する。7節で説明した通り、実際に使われていないエイリアスはこの後の`imports.Process`が自動的に取り除く。

`-o`が省略された場合の出力先は`deriveOutputPath`が決める。`filepath.Ext`でIRファイルパスの拡張子を判定し、`.go`に置き換える(拡張子が無ければ`.go`を付け足す)だけの単純な関数。

```go
func deriveOutputPath(irPath string) string {
    ext := filepath.Ext(irPath)
    if ext == "" {
        return irPath + ".go"
    }
    return strings.TrimSuffix(irPath, ext) + ".go"
}
```

## 設計上の要点(まとめ)

- **トークンの分類(classify)を、命令が何かを判定するより先に済ませる**。`tokenizeAndClassify`が行全体を一括で`[]Atom`にし、以降の命令別パース処理は分類済みデータを読むだけになる
- **「命令名の判定(緩い)」→「引数のカテゴリ照合(厳密)」の2段階**。前者は`switch`、後者は`atomExpr`/`checkKind`
- **識別子(`$`/`&`/`%`/`@`)の複合形を専用命令(`ASET`/`AGET`/`PSET`/`PGET`/`ADDR`/`SLICE`)へ分離**したことで、Kindの総数が減り`classify`・`Category`の両方が単純化された
- **検証だけが必要な箇所(ラベル・宣言名など)は`checkKind`、式を組み立てたい箇所は`atomExpr`と明確に使い分ける**。両者を混同すると、`funcName`が無いと組み立てられない形式や、`atomToExpr`にKind分岐が無い形式(ラベル)で予期せぬ失敗を起こす
- **コンテナ型(チャネル/スライス/マップ/構造体/関数型)は`TYPE`系命令による`deftype`事前宣言に統一**し、型のインライン埋め込みを廃止した
- **`:`区切りの分割ロジックを`splitColon`に集約**し、`CALL`/`FUNC`/`FNTYPE`/`CLOS`で共有する
- **`LABEL`は次の行が何であるかに関わらず常に`label: ;`を生成する仕様にした**ことで、「次の行を先読みして挙動を変える」処理が不要になり、`LABEL`は他の1行命令と全く同じに扱える
- **意味の正しさの検証は一切自前で持たず、`go/types`に委ねる**
- **ブロック構造(`FUNC`/`SEL`/`CLOS`/`STTYPE`)は専用の走査関数で対応**し、フラットな行処理では表現できない部分だけを局所化している
- **未使用変数の救済は、命名規則による所属関数の特定(文字列分割)と、ネストしたブロックへの再帰探索を組み合わせる**ことで、`CLOS`内の変数も含めて正しい挿入位置を特定する
- **amivmコマンドの責務はGoソースファイルの出力までとし、`go build`による実行ファイル生成は行わない**。CLI引数(`-o`/`-v`/`-i`、いずれも長形式あり)の解釈(`parseArgs`)と出力パイプライン(`generateOutput`)を分離し、`verbose`フラグ1つで標準出力の有無を一括制御する

## 既知の簡略化・注意点

- `CALL`の代入先が0個の場合はGoの式文(`ExprStmt`)として扱い、戻り値を暗黙的に破棄する(Goの通常の挙動に合わせた)
- 文字列・runeリテラルのエスケープ処理は簡易的(`\"`のようなエスケープされたクォートは非対応)
- IR行番号と生成Goコードのエラー行を対応付ける仕組みは未実装
- `LABEL`は仕様上常に`;`(空文)とセットで生成されるため(5節参照)、生成コードには`label:`の次行に必ず`;`が1行入る。動作上は問題ないが、生成コードの見た目としては妥協点
- `//`によるコメントは行全体がコメントである場合のみ対応し、行末コメント(トークンの後ろに`// ...`を続ける形)は非対応
