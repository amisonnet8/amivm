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

var (
	reZero      = regexp.MustCompile(`^0$`)
	rePosInt    = regexp.MustCompile(`^[1-9]\d*$`)
	reNegInt    = regexp.MustCompile(`^-\d+$`)
	reFloatLit  = regexp.MustCompile(`^-?\d+\.\d+([eE][+-]?\d+)?$|^-?\d+[eE][+-]?\d+$`)
	reRuneLit   = regexp.MustCompile(`^'.'$`) // 簡略化: 1バイト文字のみ想定
	reStringLit = regexp.MustCompile(`^"[^"]*"$`)

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
