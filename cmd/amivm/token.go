package main

import (
	"regexp"
	"strings"
)

// =====================================================================
// トークン分類(第1層): 識別子は全て先頭のプレフィックス記号($ & % @ ^ ! ? # >)で
// 種別が決まる。識別子側には添字・スライス・デリファレンス・アドレス取得の
// 複合形は存在しない(ASET/AGET/PSET/PGET/ADDR/SLICEという専用命令に分離されている)。
// これによりKind総数は大幅に少ない。
// =====================================================================

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

	// < (メソッド名)
	KMethod

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

// Atom は分類済みのトークン。A, B, C の意味はKindごとに異なる。
type Atom struct {
	Kind    Kind
	Raw     string
	A, B, C string
}

// =====================================================================
// 正規表現(いずれもプレフィックス記号を剥がした「中身」に対して適用する)
// =====================================================================

// reBasedIntBody: 16進(0x/0X)・8進(0o/0O)・2進(0b/0B)の各整数リテラルの本体
// (符号・10進部分は含まない)。桁区切りの_は基数プレフィックスの直後、または
// 数字と数字の間にのみ許容する(先頭・末尾・_の連続は不可)。Goの整数リテラル構文
// (Go 1.13以降)を素直に真似ている。旧式の先頭0だけによる8進表記(例: 0755)は
// 0xxx/0ooo/0bbbとの視覚的な区別が付きにくく、意図的に非対応とする。
const reBasedIntBody = `0[xX]_?[0-9a-fA-F](?:_?[0-9a-fA-F])*|0[oO]_?[0-7](?:_?[0-7])*|0[bB]_?[01](?:_?[01])*`

var (
	reZero = regexp.MustCompile(`^0$`)
	// rePosInt: 10進(1-9に0-9が続く形。桁区切りの_可。先頭0は不可)、
	// または16進・8進・2進リテラル(reBasedIntBody)。
	rePosInt = regexp.MustCompile(`^(?:[1-9](?:_?[0-9])*|` + reBasedIntBody + `)$`)
	// reNegInt: 上記いずれかに-を前置したもの。10進側はrePosIntと違い先頭0を
	// 許容する(-007のような表記も従来どおり許容し、値の解釈はGo側に委ねる)。
	reNegInt   = regexp.MustCompile(`^-(?:[0-9](?:_?[0-9])*|` + reBasedIntBody + `)$`)
	reFloatLit = regexp.MustCompile(`^-?\d+\.\d+([eE][+-]?\d+)?$|^-?\d+[eE][+-]?\d+$`)
	// reRuneLit: 1文字そのまま(Unicode 1文字。Goのregexpはデフォルトでルーン単位に
	// マッチするため、非ASCII文字も1文字として扱える)、またはGoの名前付きエスケープ
	// (\a \b \f \n \r \t \v \\ \' \")、\uXXXX(コードポイント。4桁hex)、
	// \UXXXXXXXX(コードポイント。8桁hex)のいずれかを許容する。\xHH(2桁hexバイト値)・
	// 8進数バイト値エスケープ(\nnn)は、\U/\uでコードポイントを直接指定できるため
	// 意図的に非対応とする。
	reRuneLit = regexp.MustCompile(`^'(?:[^'\\]|\\[abfnrtv\\'"]|\\u[0-9A-Fa-f]{4}|\\U[0-9A-Fa-f]{8})'$`)
	// reStringLit: ダブルクォート文字列。エスケープシーケンス(\"を含む)を正しく
	// 扱うため、「エスケープされていない"以外の文字」または「\に続く任意の1文字」の
	// 繰り返しとして定義する(個々のエスケープの妥当性はgo/typesの再パースに委ねる。
	// 意味の正しさの検証はAMIVM側で持たない設計方針どおり)。
	reStringLit = regexp.MustCompile(`^"(?:[^"\\]|\\.)*"$`)

	reNumOnly         = regexp.MustCompile(`^(\d+)$`)
	reClosureParamLvl = regexp.MustCompile(`^(\d+)-(\d+)$`)

	reIdentOnly = regexp.MustCompile(`^(\w+)$`)
	reIdentSel  = regexp.MustCompile(`^(\w+)\.(\w+)$`)

	reArrTypePtrSelBody = regexp.MustCompile(`^\[(\w+)\]\*(\w+)\.(\w+)$`)
	reArrTypePtrBody    = regexp.MustCompile(`^\[(\w+)\]\*(\w+)$`)
	reArrTypeSelBody    = regexp.MustCompile(`^\[(\w+)\](\w+)\.(\w+)$`)
	reArrTypeBody       = regexp.MustCompile(`^\[(\w+)\](\w+)$`)
	reTypePtrSelBody    = regexp.MustCompile(`^\*(\w+)\.(\w+)$`)
	reTypePtrBody       = regexp.MustCompile(`^\*(\w+)$`)
	reTypeSelBody       = regexp.MustCompile(`^(\w+)\.(\w+)$`)
)

// classify はトークン1つを分類する。
// 1) 記号なしのリテラル(true/false/nil/_/:/数値/文字/文字列)を先に判定
// 2) 残った先頭の記号($ & % @ ^ ! ? # >)で名前空間を判定し、専用のサブパーサーに委譲する
func classify(tok string) Atom {
	switch {
	case tok == "true" || tok == "false":
		return Atom{Kind: KBool, Raw: tok}
	case tok == "nil":
		return Atom{Kind: KNil, Raw: tok}
	case tok == "_":
		return Atom{Kind: KBlank, Raw: tok}
	case tok == ":":
		return Atom{Kind: KColon, Raw: tok}
	}
	if reZero.MatchString(tok) {
		return Atom{Kind: KZero, Raw: tok}
	}
	if rePosInt.MatchString(tok) {
		return Atom{Kind: KPosInt, Raw: tok}
	}
	if reNegInt.MatchString(tok) {
		return Atom{Kind: KNegInt, Raw: tok}
	}
	if reFloatLit.MatchString(tok) {
		return Atom{Kind: KFloat, Raw: tok}
	}
	if reRuneLit.MatchString(tok) {
		return Atom{Kind: KRune, Raw: tok}
	}
	if reStringLit.MatchString(tok) {
		return Atom{Kind: KString, Raw: tok}
	}

	if tok == "" {
		return Atom{Kind: KInvalid, Raw: tok}
	}

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
	case '<':
		return classifyMethod(tok, tok[1:])
	default:
		return Atom{Kind: KInvalid, Raw: tok}
	}
}

func classifyParam(raw, body string) Atom {
	if m := reNumOnly.FindStringSubmatch(body); m != nil {
		return Atom{Kind: KParam, Raw: raw, A: m[1]}
	}
	return Atom{Kind: KInvalid, Raw: raw}
}

// classifyClosureParam は &N(自分がいるCLOS階層のN番目)と &L-N(階層Lを明示)の
// 両方を受け付ける。どちらもKClosureParamで、Aにパラメータ番号(N)、Bに明示された
// 階層(L。&Nの場合は空文字列で「現在の階層」を意味する)を入れる。
func classifyClosureParam(raw, body string) Atom {
	if m := reClosureParamLvl.FindStringSubmatch(body); m != nil {
		return Atom{Kind: KClosureParam, Raw: raw, A: m[2], B: m[1]}
	}
	if m := reNumOnly.FindStringSubmatch(body); m != nil {
		return Atom{Kind: KClosureParam, Raw: raw, A: m[1]}
	}
	return Atom{Kind: KInvalid, Raw: raw}
}

func classifyLocal(raw, body string) Atom {
	if m := reIdentOnly.FindStringSubmatch(body); m != nil {
		return Atom{Kind: KLocal, Raw: raw, A: m[1]}
	}
	return Atom{Kind: KInvalid, Raw: raw}
}

func classifyGlobal(raw, body string) Atom {
	if m := reIdentSel.FindStringSubmatch(body); m != nil {
		return Atom{Kind: KGlobalSel, Raw: raw, A: m[1], B: m[2]}
	}
	if m := reIdentOnly.FindStringSubmatch(body); m != nil {
		return Atom{Kind: KGlobal, Raw: raw, A: m[1]}
	}
	return Atom{Kind: KInvalid, Raw: raw}
}

func classifyField(raw, body string) Atom {
	if m := reIdentOnly.FindStringSubmatch(body); m != nil {
		return Atom{Kind: KField, Raw: raw, A: m[1]}
	}
	return Atom{Kind: KInvalid, Raw: raw}
}

func classifyMethod(raw, body string) Atom {
	if m := reIdentOnly.FindStringSubmatch(body); m != nil {
		return Atom{Kind: KMethod, Raw: raw, A: m[1]}
	}
	return Atom{Kind: KInvalid, Raw: raw}
}

func classifyType(raw, body string) Atom {
	switch {
	case reArrTypePtrSelBody.MatchString(body):
		m := reArrTypePtrSelBody.FindStringSubmatch(body)
		return Atom{Kind: KArrTypePtrSel, Raw: raw, A: m[1], B: m[2], C: m[3]}
	case reArrTypePtrBody.MatchString(body):
		m := reArrTypePtrBody.FindStringSubmatch(body)
		return Atom{Kind: KArrTypePtr, Raw: raw, A: m[1], B: m[2]}
	case reArrTypeSelBody.MatchString(body):
		m := reArrTypeSelBody.FindStringSubmatch(body)
		return Atom{Kind: KArrTypeSel, Raw: raw, A: m[1], B: m[2], C: m[3]}
	case reArrTypeBody.MatchString(body):
		m := reArrTypeBody.FindStringSubmatch(body)
		return Atom{Kind: KArrType, Raw: raw, A: m[1], B: m[2]}
	case reTypePtrSelBody.MatchString(body):
		m := reTypePtrSelBody.FindStringSubmatch(body)
		return Atom{Kind: KTypePtrSel, Raw: raw, A: m[1], B: m[2]}
	case reTypePtrBody.MatchString(body):
		m := reTypePtrBody.FindStringSubmatch(body)
		return Atom{Kind: KTypePtr, Raw: raw, A: m[1]}
	case reTypeSelBody.MatchString(body):
		m := reTypeSelBody.FindStringSubmatch(body)
		return Atom{Kind: KTypeSel, Raw: raw, A: m[1], B: m[2]}
	case reIdentOnly.MatchString(body):
		return Atom{Kind: KType, Raw: raw, A: body}
	default:
		return Atom{Kind: KInvalid, Raw: raw}
	}
}

func classifyAmivmFunc(raw, body string) Atom {
	if body == "main" {
		return Atom{Kind: KAmivmMain, Raw: raw}
	}
	if reIdentOnly.MatchString(body) {
		return Atom{Kind: KAmivmFunc, Raw: raw, A: body}
	}
	return Atom{Kind: KInvalid, Raw: raw}
}

func classifyGoFunc(raw, body string) Atom {
	if m := reIdentSel.FindStringSubmatch(body); m != nil {
		return Atom{Kind: KGoFuncSel, Raw: raw, A: m[1], B: m[2]}
	}
	if reIdentOnly.MatchString(body) {
		return Atom{Kind: KGoFunc, Raw: raw, A: body}
	}
	return Atom{Kind: KInvalid, Raw: raw}
}

func classifyLabel(raw, body string) Atom {
	if reIdentOnly.MatchString(body) {
		return Atom{Kind: KLabel, Raw: raw, A: body}
	}
	return Atom{Kind: KInvalid, Raw: raw}
}

// =====================================================================
// トークナイズ(タブ区切りで分割し、その場でclassifyする)
// =====================================================================

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

func keyword(line string) string {
	atoms := tokenizeAndClassify(line)
	if len(atoms) == 0 {
		return ""
	}
	return atoms[0].Raw
}

// splitLinesTrimmed は行頭・行末の空白(タブによるインデント含む)を取り除き、
// 空行と//で始まるコメント行を除外する。
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
	if rest := strings.TrimSpace(source[start:]); rest != "" && !strings.HasPrefix(rest, "//") {
		lines = append(lines, rest)
	}
	return lines
}
