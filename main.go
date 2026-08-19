package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/imports"
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

	reNumOnly = regexp.MustCompile(`^(\d+)$`)

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

func classifyClosureParam(raw, body string) Atom {
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

// =====================================================================
// AST組み立て(第2層)
// =====================================================================

// amivmFuncGoName は AMIVM内定義関数のGo側の宣言名。mainだけ例外でそのまま"main"。
func amivmFuncGoName(name string) string {
	if name == "main" {
		return "main"
	}
	return name + "_amivm_function"
}

// amivmParamGoName / amivmLocalGoName は、関数内変数・パラメータのGo側の実名。
// funcNameは常にAMIVM側の素の関数名("main"含む)を渡す(Go宣言名のamivmFuncGoNameとは別の変換)。
func amivmParamGoName(funcName string, n int) string {
	return fmt.Sprintf("%s_amivm_function_param%d", funcName, n)
}
func amivmLocalGoName(funcName, varName string) string {
	return funcName + "_amivm_function_" + varName
}

// closureParamGoName はクロージャー引数のGo側の実名。関数名による修飾はしない
// (& N → amivm_closure_paramN で常に固定。func literalごとに別スコープになるため衝突しない)。
func closureParamGoName(n int) string {
	return fmt.Sprintf("amivm_closure_param%d", n)
}

func paramBaseExpr(funcName, numStr string) (ast.Expr, error) {
	if funcName == "" {
		return nil, fmt.Errorf("$%sは関数定義の外では使えません", numStr)
	}
	n, _ := strconv.Atoi(numStr)
	return ast.NewIdent(amivmParamGoName(funcName, n)), nil
}

func closureParamBaseExpr(numStr string) ast.Expr {
	n, _ := strconv.Atoi(numStr)
	return ast.NewIdent(closureParamGoName(n))
}

func localBaseExpr(funcName, name string) (ast.Expr, error) {
	if funcName == "" {
		return nil, fmt.Errorf("%%%sは関数定義の外では使えません", name)
	}
	return ast.NewIdent(amivmLocalGoName(funcName, name)), nil
}

func globalBaseExpr(name string) ast.Expr { return ast.NewIdent(name) }

func globalSelBaseExpr(pkg, member string) ast.Expr {
	return &ast.SelectorExpr{X: ast.NewIdent(pkg), Sel: ast.NewIdent(member)}
}

func arrayTypeExpr(sizeStr string, elt ast.Expr) (ast.Expr, error) {
	n, err := strconv.Atoi(sizeStr)
	if err != nil {
		return nil, fmt.Errorf("配列サイズが不正です: %s", sizeStr)
	}
	return &ast.ArrayType{Len: &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(n)}, Elt: elt}, nil
}

func sliceTypeExpr(elt ast.Expr) ast.Expr {
	// Goのスライス型はast.ArrayTypeでLenをnilにしたもの
	return &ast.ArrayType{Len: nil, Elt: elt}
}

func chanTypeExpr(elt ast.Expr) ast.Expr {
	return &ast.ChanType{Dir: ast.SEND | ast.RECV, Value: elt}
}

func atomToExpr(a Atom, funcName string) (ast.Expr, error) {
	switch a.Kind {
	case KBlank:
		return ast.NewIdent("_"), nil
	case KBool, KNil:
		return ast.NewIdent(a.Raw), nil
	case KZero, KPosInt, KNegInt:
		return &ast.BasicLit{Kind: token.INT, Value: a.Raw}, nil
	case KFloat:
		return &ast.BasicLit{Kind: token.FLOAT, Value: a.Raw}, nil
	case KString:
		return &ast.BasicLit{Kind: token.STRING, Value: a.Raw}, nil
	case KRune:
		return &ast.BasicLit{Kind: token.CHAR, Value: a.Raw}, nil

	case KParam:
		return paramBaseExpr(funcName, a.A)
	case KClosureParam:
		return closureParamBaseExpr(a.A), nil
	case KLocal:
		return localBaseExpr(funcName, a.A)
	case KGlobal:
		return globalBaseExpr(a.A), nil
	case KGlobalSel:
		return globalSelBaseExpr(a.A, a.B), nil

	case KField:
		return ast.NewIdent(a.A), nil

	case KType:
		return ast.NewIdent(a.A), nil
	case KTypeSel:
		return &ast.SelectorExpr{X: ast.NewIdent(a.A), Sel: ast.NewIdent(a.B)}, nil
	case KTypePtr:
		return &ast.StarExpr{X: ast.NewIdent(a.A)}, nil
	case KTypePtrSel:
		return &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent(a.A), Sel: ast.NewIdent(a.B)}}, nil
	case KArrType:
		return arrayTypeExpr(a.A, ast.NewIdent(a.B))
	case KArrTypeSel:
		return arrayTypeExpr(a.A, &ast.SelectorExpr{X: ast.NewIdent(a.B), Sel: ast.NewIdent(a.C)})
	case KArrTypePtr:
		return arrayTypeExpr(a.A, &ast.StarExpr{X: ast.NewIdent(a.B)})
	case KArrTypePtrSel:
		return arrayTypeExpr(a.A, &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent(a.B), Sel: ast.NewIdent(a.C)}})

	case KAmivmFunc:
		return ast.NewIdent(amivmFuncGoName(a.A)), nil
	case KAmivmMain:
		return ast.NewIdent("main"), nil
	case KGoFunc:
		return ast.NewIdent(a.A), nil
	case KGoFuncSel:
		return &ast.SelectorExpr{X: ast.NewIdent(a.A), Sel: ast.NewIdent(a.B)}, nil

	default:
		return nil, fmt.Errorf("解釈できない形式です: %s", a.Raw)
	}
}

// labelGoName はKLabelのAtomからGoの識別子として妥当なラベル名(#を除いた部分)を取り出す。
func labelGoName(a Atom) (string, error) {
	if a.Kind != KLabel {
		return "", fmt.Errorf("ラベルにこの形式は使えません: %s", a.Raw)
	}
	return a.A, nil
}

// amivmFuncNameOf は FUNC の定義名Atom(KAmivmFunc/KAmivmMain)から、
// パラメータ・ローカル変数の命名に使う「素の関数名」を取り出す。
func amivmFuncNameOf(a Atom) (string, error) {
	switch a.Kind {
	case KAmivmMain:
		return "main", nil
	case KAmivmFunc:
		return a.A, nil
	default:
		return "", fmt.Errorf("関数名にこの形式は使えません: %s", a.Raw)
	}
}

// =====================================================================
// オペランドカテゴリ(第3層)
// =====================================================================

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

var categoryLabel = map[Category]string{
	CatVa: "VAR変数名", CatGv: "GVAR変数名",
	CatSingle: "単一左辺/チャネル変数", CatMulti: "複数左辺",
	CatVariable: "変数参照", CatType: "型", CatDeftype: "定義型",
	CatField: "構造体フィールド名",
	CatWhole: "0以上の整数", CatFromTo: "スライス範囲(from/to)",
	CatInt: "整数", CatNumber: "数値", CatBool: "真偽値",
	CatSlice: "スライス/文字列", CatOrder: "順序比較値", CatValue: "値",
	CatDefname: "関数定義名", CatCallname: "呼び出し対象", CatLabel: "ラベル名",
}

func kindSet(kinds ...Kind) map[Kind]bool {
	m := make(map[Kind]bool, len(kinds))
	for _, k := range kinds {
		m[k] = true
	}
	return m
}

func mergeKinds(sets ...map[Kind]bool) map[Kind]bool {
	m := map[Kind]bool{}
	for _, s := range sets {
		for k := range s {
			m[k] = true
		}
	}
	return m
}

var (
	// identRefFull: $ / & / % / @(プレーン・セレクタ)の単純参照。
	identRefFull = kindSet(KParam, KClosureParam, KLocal, KGlobal, KGlobalSel)
	// identRefNoSel: identRefFullから@xxx.yyy(他パッケージ参照)を除いたもの。
	// 「single」「multi」カテゴリで使う。
	identRefNoSel = kindSet(KParam, KClosureParam, KLocal, KGlobal)

	typeKindsAll = kindSet(
		KType, KTypeSel, KTypePtr, KTypePtrSel,
		KArrType, KArrTypeSel, KArrTypePtr, KArrTypePtrSel,
	)
)

var allowedKinds = buildAllowedKinds()

func buildAllowedKinds() map[Category]map[Kind]bool {
	m := map[Category]map[Kind]bool{}

	m[CatVa] = kindSet(KLocal)
	m[CatGv] = kindSet(KGlobal)
	m[CatSingle] = identRefNoSel
	m[CatMulti] = mergeKinds(identRefNoSel, kindSet(KBlank))
	m[CatVariable] = identRefFull

	m[CatType] = typeKindsAll
	m[CatDeftype] = kindSet(KType)
	m[CatField] = kindSet(KField)

	whole := mergeKinds(identRefFull, kindSet(KZero, KPosInt, KRune))
	m[CatWhole] = whole
	m[CatFromTo] = mergeKinds(whole, kindSet(KBlank))

	integer := mergeKinds(whole, kindSet(KNegInt))
	m[CatInt] = integer

	number := mergeKinds(integer, kindSet(KFloat))
	m[CatNumber] = number

	boolean := mergeKinds(identRefFull, kindSet(KBool))
	m[CatBool] = boolean

	slice := mergeKinds(identRefFull, kindSet(KString))
	m[CatSlice] = slice

	order := mergeKinds(identRefFull, kindSet(KZero, KPosInt, KNegInt, KFloat, KRune, KString))
	m[CatOrder] = order

	value := mergeKinds(order, kindSet(KBool, KNil))
	m[CatValue] = value

	m[CatDefname] = kindSet(KAmivmFunc, KAmivmMain)
	m[CatCallname] = kindSet(KAmivmFunc, KAmivmMain, KGoFunc, KGoFuncSel, KLocal)
	m[CatLabel] = kindSet(KLabel)

	return m
}

func atomExpr(a Atom, funcName string, cat Category) (ast.Expr, error) {
	if err := checkKind(a, cat); err != nil {
		return nil, err
	}
	return atomToExpr(a, funcName)
}

// checkKind はKindがカテゴリの許容集合に属するかだけを検証する。ast.Exprを組み立てない
// ため、$N/%xxxのようにfuncNameがないと組み立てられない形式や、KLabelのように
// atomToExprが扱わない(ラベルはlabelGoNameで別途名前を取り出すため)形式でも、
// 「検証だけしたい」箇所(VAR名・GVAR名・ラベル・関数定義名・deftype名の妥当性チェックなど)
// で安全に使える。
func checkKind(a Atom, cat Category) error {
	if a.Kind == KInvalid {
		return fmt.Errorf("%sとして解釈できない形式です: %s", categoryLabel[cat], a.Raw)
	}
	if !allowedKinds[cat][a.Kind] {
		return fmt.Errorf("%sにこの形式は使えません: %s", categoryLabel[cat], a.Raw)
	}
	return nil
}

// =====================================================================
// 各命令のパース
// =====================================================================

func joinRaw(atoms []Atom) string {
	raws := make([]string, len(atoms))
	for i, a := range atoms {
		raws[i] = a.Raw
	}
	return strings.Join(raws, " ")
}

func expectArgs(kw string, atoms []Atom, n int) error {
	if len(atoms) != n {
		return fmt.Errorf("%s構文が不正です(引数の数が合いません): %s %s", kw, kw, joinRaw(atoms))
	}
	return nil
}

// splitColon は、CALL/FUNC/FNTYPE/CLOSで使う「:」1個による2分割を行う。
// コロンは戻り値・代入先と、呼び出し対象・パラメータ列を区切るために必ず1個必要。
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

// parseSingleLine は、ブロック開始行(FUNC/SEL/CLOS/STTYPE)・終了行・
// LABELを除いた、単一行で完結する命令をパースする。GVARはここでは扱わない
// (トップレベル専用のため、buildProgramでのみ処理される)。
func parseSingleLine(line, funcName string) (ast.Stmt, error) {
	atoms := tokenizeAndClassify(line)
	if len(atoms) == 0 {
		return nil, nil
	}
	kw := atoms[0].Raw
	rest := atoms[1:]

	switch kw {
	case "VAR":
		return parseVar(rest, funcName)
	case "SET":
		return parseSet(rest, funcName)
	case "ASET":
		return parseAset(rest, funcName)
	case "AGET":
		return parseAget(rest, funcName)
	case "PSET":
		return parsePset(rest, funcName)
	case "PGET":
		return parsePget(rest, funcName)
	case "ADDR":
		return parseAddr(rest, funcName)
	case "ADD", "SUB", "MUL", "DIV", "MOD", "BAND", "BOR", "BXOR", "BCLEAR", "SHL", "SHR", "AND", "OR",
		"EQ", "NEQ", "LT", "LTE", "GT", "GTE":
		return parseBin2(kw, rest, funcName)
	case "BNOT", "NOT":
		return parseUnary(kw, rest, funcName)
	case "LABEL":
		return parseLabel(rest)
	case "GOTO":
		return parseGoto(rest)
	case "IF":
		return parseIf(rest, funcName)
	case "RET":
		return parseRet(rest, funcName)
	case "CALL":
		return parseCall(rest, funcName)
	case "DEFER":
		return parseDeferOrSpawn("DEFER", rest, funcName)
	case "SPAWN":
		return parseDeferOrSpawn("SPAWN", rest, funcName)
	case "CHMAKE":
		return parseChOrSlMake("CHMAKE", rest, funcName)
	case "SLMAKE":
		return parseChOrSlMake("SLMAKE", rest, funcName)
	case "MPMAKE":
		return parseMpMake(rest, funcName)
	case "CHSEND":
		return parseChSend(rest, funcName)
	case "CHRECV":
		return parseChRecv(rest, funcName)
	case "CONCAT":
		return parseConcat(rest, funcName)
	case "SLICE":
		return parseSlice(rest, funcName)
	case "FSET":
		return parseFset(rest, funcName)
	case "FGET":
		return parseFget(rest, funcName)
	case "MSET":
		return parseMset(rest, funcName)
	case "MGET":
		return parseMget(rest, funcName)
	default:
		return nil, fmt.Errorf("未知の命令です: %s", line)
	}
}

func parseVar(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("VAR", atoms, 2); err != nil {
		return nil, err
	}
	if funcName == "" {
		return nil, fmt.Errorf("VARは関数内でのみ使えます(トップレベルはGVARを使ってください)")
	}
	if err := checkKind(atoms[0], CatVa); err != nil {
		return nil, fmt.Errorf("VARの変数名が不正です: %w", err)
	}
	typExpr, err := atomExpr(atoms[1], "", CatType)
	if err != nil {
		return nil, fmt.Errorf("VARの型が不正です: %w", err)
	}
	goName := amivmLocalGoName(funcName, atoms[0].A)
	return &ast.DeclStmt{
		Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{
				&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(goName)}, Type: typExpr},
			},
		},
	}, nil
}

func parseGvar(atoms []Atom) (ast.Decl, error) {
	if err := expectArgs("GVAR", atoms, 2); err != nil {
		return nil, err
	}
	if err := checkKind(atoms[0], CatGv); err != nil {
		return nil, fmt.Errorf("GVARの変数名が不正です: %w", err)
	}
	typExpr, err := atomExpr(atoms[1], "", CatType)
	if err != nil {
		return nil, fmt.Errorf("GVARの型が不正です: %w", err)
	}
	return &ast.GenDecl{
		Tok: token.VAR,
		Specs: []ast.Spec{
			&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(atoms[0].A)}, Type: typExpr},
		},
	}, nil
}

func parseSet(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("SET", atoms, 2); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("SETの左辺が不正です: %w", err)
	}
	rhs, err := atomExpr(atoms[1], funcName, CatValue)
	if err != nil {
		return nil, fmt.Errorf("SETの右辺が不正です: %w", err)
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{rhs}}, nil
}

func parseAset(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("ASET", atoms, 3); err != nil {
		return nil, err
	}
	arr, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("ASETの配列が不正です: %w", err)
	}
	idx, err := atomExpr(atoms[1], funcName, CatWhole)
	if err != nil {
		return nil, fmt.Errorf("ASETの添字が不正です: %w", err)
	}
	val, err := atomExpr(atoms[2], funcName, CatValue)
	if err != nil {
		return nil, fmt.Errorf("ASETの値が不正です: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.IndexExpr{X: arr, Index: idx}}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{val},
	}, nil
}

func parseAget(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("AGET", atoms, 3); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("AGETの代入先が不正です: %w", err)
	}
	variable, err := atomExpr(atoms[1], funcName, CatVariable)
	if err != nil {
		return nil, fmt.Errorf("AGETの配列が不正です: %w", err)
	}
	idx, err := atomExpr(atoms[2], funcName, CatWhole)
	if err != nil {
		return nil, fmt.Errorf("AGETの添字が不正です: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.IndexExpr{X: variable, Index: idx}},
	}, nil
}

func parsePset(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("PSET", atoms, 2); err != nil {
		return nil, err
	}
	ptr, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("PSETのポインタが不正です: %w", err)
	}
	val, err := atomExpr(atoms[1], funcName, CatValue)
	if err != nil {
		return nil, fmt.Errorf("PSETの値が不正です: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.StarExpr{X: ptr}}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{val},
	}, nil
}

func parsePget(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("PGET", atoms, 2); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("PGETの代入先が不正です: %w", err)
	}
	variable, err := atomExpr(atoms[1], funcName, CatVariable)
	if err != nil {
		return nil, fmt.Errorf("PGETのポインタが不正です: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.StarExpr{X: variable}},
	}, nil
}

func parseAddr(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("ADDR", atoms, 2); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("ADDRの代入先が不正です: %w", err)
	}
	variable, err := atomExpr(atoms[1], funcName, CatVariable)
	if err != nil {
		return nil, fmt.Errorf("ADDRの対象が不正です: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: variable}},
	}, nil
}

var binOpTokens = map[string]token.Token{
	"ADD": token.ADD, "SUB": token.SUB, "MUL": token.MUL, "DIV": token.QUO,
	"MOD": token.REM, "BAND": token.AND, "BOR": token.OR, "BXOR": token.XOR,
	"BCLEAR": token.AND_NOT, "SHL": token.SHL, "SHR": token.SHR,
	"AND": token.LAND, "OR": token.LOR,
	"EQ": token.EQL, "NEQ": token.NEQ, "LT": token.LSS, "LTE": token.LEQ,
	"GT": token.GTR, "GTE": token.GEQ,
}

var binOperandCategories = map[string][2]Category{
	"ADD": {CatNumber, CatNumber}, "SUB": {CatNumber, CatNumber},
	"MUL": {CatNumber, CatNumber}, "DIV": {CatNumber, CatNumber},
	"MOD": {CatInt, CatInt}, "BAND": {CatInt, CatInt}, "BOR": {CatInt, CatInt},
	"BXOR": {CatInt, CatInt}, "BCLEAR": {CatInt, CatInt},
	"SHL": {CatInt, CatWhole}, "SHR": {CatInt, CatWhole},
	"AND": {CatBool, CatBool}, "OR": {CatBool, CatBool},
	"EQ": {CatValue, CatValue}, "NEQ": {CatValue, CatValue},
	"LT": {CatOrder, CatOrder}, "LTE": {CatOrder, CatOrder},
	"GT": {CatOrder, CatOrder}, "GTE": {CatOrder, CatOrder},
}

func parseBin2(kw string, atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs(kw, atoms, 3); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("%sの左辺が不正です: %w", kw, err)
	}
	cats := binOperandCategories[kw]
	a, err := atomExpr(atoms[1], funcName, cats[0])
	if err != nil {
		return nil, fmt.Errorf("%sの第1オペランドが不正です: %w", kw, err)
	}
	b, err := atomExpr(atoms[2], funcName, cats[1])
	if err != nil {
		return nil, fmt.Errorf("%sの第2オペランドが不正です: %w", kw, err)
	}
	op := binOpTokens[kw]
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.BinaryExpr{X: a, Op: op, Y: b}},
	}, nil
}

func parseUnary(kw string, atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs(kw, atoms, 2); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("%sの左辺が不正です: %w", kw, err)
	}
	cat := CatBool
	op := token.NOT
	if kw == "BNOT" {
		cat = CatInt
		op = token.XOR
	}
	a, err := atomExpr(atoms[1], funcName, cat)
	if err != nil {
		return nil, fmt.Errorf("%sのオペランドが不正です: %w", kw, err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.UnaryExpr{Op: op, X: a}},
	}, nil
}

// parseLabel は「LABEL label → label: ;」を解釈する。ラベルは常に空文とセットで
// 1行完結するため(IR.txt参照)、直後の行の先読み・合体は不要。
func parseLabel(atoms []Atom) (ast.Stmt, error) {
	if err := expectArgs("LABEL", atoms, 1); err != nil {
		return nil, err
	}
	if err := checkKind(atoms[0], CatLabel); err != nil {
		return nil, fmt.Errorf("LABEL名が不正です: %w", err)
	}
	name, err := labelGoName(atoms[0])
	if err != nil {
		return nil, err
	}
	return &ast.LabeledStmt{Label: ast.NewIdent(name), Stmt: &ast.EmptyStmt{}}, nil
}

func parseGoto(atoms []Atom) (ast.Stmt, error) {
	if err := expectArgs("GOTO", atoms, 1); err != nil {
		return nil, err
	}
	if err := checkKind(atoms[0], CatLabel); err != nil {
		return nil, fmt.Errorf("GOTOのラベルが不正です: %w", err)
	}
	name, err := labelGoName(atoms[0])
	if err != nil {
		return nil, err
	}
	return gotoStmt(name), nil
}

func gotoStmt(label string) ast.Stmt {
	return &ast.BranchStmt{Tok: token.GOTO, Label: ast.NewIdent(label)}
}

func parseIf(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("IF", atoms, 2); err != nil {
		return nil, err
	}
	cond, err := atomExpr(atoms[0], funcName, CatBool)
	if err != nil {
		return nil, fmt.Errorf("IFの条件が不正です: %w", err)
	}
	if err := checkKind(atoms[1], CatLabel); err != nil {
		return nil, fmt.Errorf("IFのラベルが不正です: %w", err)
	}
	name, err := labelGoName(atoms[1])
	if err != nil {
		return nil, err
	}
	return &ast.IfStmt{
		Cond: cond,
		Body: &ast.BlockStmt{List: []ast.Stmt{gotoStmt(name)}},
	}, nil
}

func parseRet(atoms []Atom, funcName string) (ast.Stmt, error) {
	var results []ast.Expr
	for _, a := range atoms {
		v, err := atomExpr(a, funcName, CatValue)
		if err != nil {
			return nil, fmt.Errorf("RETの戻り値が不正です: %w", err)
		}
		results = append(results, v)
	}
	return &ast.ReturnStmt{Results: results}, nil
}

func buildCallExpr(callAtom Atom, argAtoms []Atom, funcName string) (*ast.CallExpr, error) {
	fn, err := atomExpr(callAtom, funcName, CatCallname)
	if err != nil {
		return nil, fmt.Errorf("呼び出し対象が不正です: %w", err)
	}
	var args []ast.Expr
	for _, a := range argAtoms {
		v, err := atomExpr(a, funcName, CatValue)
		if err != nil {
			return nil, fmt.Errorf("引数が不正です: %w", err)
		}
		args = append(args, v)
	}
	return &ast.CallExpr{Fun: fn, Args: args}, nil
}

// parseCall は「CALL multi1 multi2 ... : callname value1 value2 ...」を解釈する。
// コロンの左側(多重代入先)は空でもよい(その場合は式文として呼び出すだけになる)。
func parseCall(atoms []Atom, funcName string) (ast.Stmt, error) {
	destAtoms, rest, err := splitColon(atoms)
	if err != nil {
		return nil, fmt.Errorf("CALL構文が不正です: %w", err)
	}
	if len(rest) == 0 {
		return nil, fmt.Errorf("CALL構文が不正です(呼び出し対象がありません): %s", joinRaw(atoms))
	}
	callAtom, argAtoms := rest[0], rest[1:]

	callExpr, err := buildCallExpr(callAtom, argAtoms, funcName)
	if err != nil {
		return nil, err
	}

	if len(destAtoms) == 0 {
		return &ast.ExprStmt{X: callExpr}, nil
	}

	var lhs []ast.Expr
	for _, d := range destAtoms {
		e, err := atomExpr(d, funcName, CatMulti)
		if err != nil {
			return nil, fmt.Errorf("CALLの代入先が不正です: %w", err)
		}
		lhs = append(lhs, e)
	}
	return &ast.AssignStmt{Lhs: lhs, Tok: token.ASSIGN, Rhs: []ast.Expr{callExpr}}, nil
}

// parseDeferOrSpawn は「DEFER/SPAWN callname value1 value2 ...」を解釈する(コロンなし)。
func parseDeferOrSpawn(kw string, atoms []Atom, funcName string) (ast.Stmt, error) {
	if len(atoms) == 0 {
		return nil, fmt.Errorf("%s構文が不正です(呼び出し対象がありません): %s", kw, kw)
	}
	callExpr, err := buildCallExpr(atoms[0], atoms[1:], funcName)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", kw, err)
	}
	if kw == "DEFER" {
		return &ast.DeferStmt{Call: callExpr}, nil
	}
	return &ast.GoStmt{Call: callExpr}, nil
}

func parseChOrSlMake(kw string, atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs(kw, atoms, 3); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("%sの変数が不正です: %w", kw, err)
	}
	typExpr, err := atomExpr(atoms[1], "", CatDeftype)
	if err != nil {
		return nil, fmt.Errorf("%sの型が不正です: %w", kw, err)
	}
	size, err := atomExpr(atoms[2], funcName, CatWhole)
	if err != nil {
		return nil, fmt.Errorf("%sのサイズが不正です: %w", kw, err)
	}
	makeCall := &ast.CallExpr{Fun: ast.NewIdent("make"), Args: []ast.Expr{typExpr, size}}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{makeCall},
	}, nil
}

func parseMpMake(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("MPMAKE", atoms, 2); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("MPMAKEの変数が不正です: %w", err)
	}
	typExpr, err := atomExpr(atoms[1], "", CatDeftype)
	if err != nil {
		return nil, fmt.Errorf("MPMAKEの型が不正です: %w", err)
	}
	makeCall := &ast.CallExpr{Fun: ast.NewIdent("make"), Args: []ast.Expr{typExpr}}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{makeCall},
	}, nil
}

func parseChSend(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("CHSEND", atoms, 2); err != nil {
		return nil, err
	}
	ch, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("CHSENDのチャネルが不正です: %w", err)
	}
	v, err := atomExpr(atoms[1], funcName, CatValue)
	if err != nil {
		return nil, fmt.Errorf("CHSENDの送信値が不正です: %w", err)
	}
	return &ast.SendStmt{Chan: ch, Value: v}, nil
}

func parseChRecv(atoms []Atom, funcName string) (ast.Stmt, error) {
	if len(atoms) < 2 || len(atoms) > 3 {
		return nil, fmt.Errorf("CHRECV構文が不正です(書式: CHRECV l1 (l2) cs): %s", joinRaw(atoms))
	}
	destAtoms := atoms[:len(atoms)-1]
	chAtom := atoms[len(atoms)-1]

	var lhs []ast.Expr
	for _, d := range destAtoms {
		e, err := atomExpr(d, funcName, CatMulti)
		if err != nil {
			return nil, fmt.Errorf("CHRECVの代入先が不正です: %w", err)
		}
		lhs = append(lhs, e)
	}
	ch, err := atomExpr(chAtom, funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("CHRECVのチャネルが不正です: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: lhs, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.ARROW, X: ch}},
	}, nil
}

func parseConcat(atoms []Atom, funcName string) (ast.Stmt, error) {
	if len(atoms) < 3 {
		return nil, fmt.Errorf("CONCAT構文が不正です(書式: CONCAT l1 s1 s2 ...): %s", joinRaw(atoms))
	}
	lhs, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("CONCATの左辺が不正です: %w", err)
	}
	var parts []ast.Expr
	for _, a := range atoms[1:] {
		v, err := atomExpr(a, funcName, CatSlice)
		if err != nil {
			return nil, fmt.Errorf("CONCATのオペランドが不正です: %w", err)
		}
		parts = append(parts, v)
	}
	expr := parts[0]
	for _, p := range parts[1:] {
		expr = &ast.BinaryExpr{X: expr, Op: token.ADD, Y: p}
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{expr}}, nil
}

func parseSlice(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("SLICE", atoms, 4); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("SLICEの代入先が不正です: %w", err)
	}
	src, err := atomExpr(atoms[1], funcName, CatSlice)
	if err != nil {
		return nil, fmt.Errorf("SLICEの対象が不正です: %w", err)
	}
	var low, high ast.Expr
	if _, err := atomExpr(atoms[2], funcName, CatFromTo); err != nil {
		return nil, fmt.Errorf("SLICEのfromが不正です: %w", err)
	}
	if atoms[2].Kind != KBlank {
		low, _ = atomToExpr(atoms[2], funcName)
	}
	if _, err := atomExpr(atoms[3], funcName, CatFromTo); err != nil {
		return nil, fmt.Errorf("SLICEのtoが不正です: %w", err)
	}
	if atoms[3].Kind != KBlank {
		high, _ = atomToExpr(atoms[3], funcName)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.SliceExpr{X: src, Low: low, High: high}},
	}, nil
}

func parseFset(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("FSET", atoms, 3); err != nil {
		return nil, err
	}
	obj, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("FSETの対象が不正です: %w", err)
	}
	if atoms[1].Kind != KField {
		return nil, fmt.Errorf("FSETのフィールド名が不正です: %s", atoms[1].Raw)
	}
	val, err := atomExpr(atoms[2], funcName, CatValue)
	if err != nil {
		return nil, fmt.Errorf("FSETの値が不正です: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.SelectorExpr{X: obj, Sel: ast.NewIdent(atoms[1].A)}}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{val},
	}, nil
}

func parseFget(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("FGET", atoms, 3); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("FGETの代入先が不正です: %w", err)
	}
	variable, err := atomExpr(atoms[1], funcName, CatVariable)
	if err != nil {
		return nil, fmt.Errorf("FGETの対象が不正です: %w", err)
	}
	if atoms[2].Kind != KField {
		return nil, fmt.Errorf("FGETのフィールド名が不正です: %s", atoms[2].Raw)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.SelectorExpr{X: variable, Sel: ast.NewIdent(atoms[2].A)}},
	}, nil
}

func parseMset(atoms []Atom, funcName string) (ast.Stmt, error) {
	if err := expectArgs("MSET", atoms, 3); err != nil {
		return nil, err
	}
	m, err := atomExpr(atoms[0], funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("MSETのmapが不正です: %w", err)
	}
	key, err := atomExpr(atoms[1], funcName, CatValue)
	if err != nil {
		return nil, fmt.Errorf("MSETのキーが不正です: %w", err)
	}
	val, err := atomExpr(atoms[2], funcName, CatValue)
	if err != nil {
		return nil, fmt.Errorf("MSETの値が不正です: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.IndexExpr{X: m, Index: key}}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{val},
	}, nil
}

func parseMget(atoms []Atom, funcName string) (ast.Stmt, error) {
	if len(atoms) < 3 || len(atoms) > 4 {
		return nil, fmt.Errorf("MGET構文が不正です(書式: MGET l1 (l2) m v1): %s", joinRaw(atoms))
	}
	valueAtom := atoms[len(atoms)-1]
	mapAtom := atoms[len(atoms)-2]
	destAtoms := atoms[:len(atoms)-2]

	var lhs []ast.Expr
	for _, d := range destAtoms {
		e, err := atomExpr(d, funcName, CatMulti)
		if err != nil {
			return nil, fmt.Errorf("MGETの代入先が不正です: %w", err)
		}
		lhs = append(lhs, e)
	}
	m, err := atomExpr(mapAtom, funcName, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("MGETのmapが不正です: %w", err)
	}
	key, err := atomExpr(valueAtom, funcName, CatValue)
	if err != nil {
		return nil, fmt.Errorf("MGETのキーが不正です: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: lhs, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.IndexExpr{X: m, Index: key}},
	}, nil
}

// =====================================================================
// ブロック構造(FUNC/ENDFUNC, SEL/ENDSEL, CLOS/ENDCLOS, STTYPE/ENDSTTYPE)と
// LABELの先読み合体
// =====================================================================

// typeFieldsUnnamed は型トークン列を無名の*ast.Fieldに変換する(FNTYPEのパラメータ・
// 戻り値、FUNCの戻り値で使う)。
func typeFieldsUnnamed(atoms []Atom) ([]*ast.Field, error) {
	var fields []*ast.Field
	for _, a := range atoms {
		te, err := atomExpr(a, "", CatType)
		if err != nil {
			return nil, err
		}
		fields = append(fields, &ast.Field{Type: te})
	}
	return fields, nil
}

// typeFieldsNamed は型トークン列を、namerが生成する名前付きの*ast.Fieldに変換する
// (FUNCのパラメータ、CLOSのパラメータで使う)。
func typeFieldsNamed(atoms []Atom, namer func(i int) string) ([]*ast.Field, error) {
	var fields []*ast.Field
	for i, a := range atoms {
		te, err := atomExpr(a, "", CatType)
		if err != nil {
			return nil, err
		}
		fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(namer(i))}, Type: te})
	}
	return fields, nil
}

func fieldListOrNil(fields []*ast.Field) *ast.FieldList {
	if len(fields) == 0 {
		return nil
	}
	return &ast.FieldList{List: fields}
}

func typeDecl(name string, typ ast.Expr) ast.Decl {
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{Name: ast.NewIdent(name), Type: typ},
		},
	}
}

// parseFuncSignature は「FUNC defname type1 type2 ... : type3 type4 ...」を解釈する。
// defnameの後、コロンより前がパラメータ型、コロンより後が戻り値型。
func parseFuncSignature(line string) (defName string, params []*ast.Field, results []*ast.Field, err error) {
	atoms := tokenizeAndClassify(line)
	if len(atoms) == 0 || atoms[0].Raw != "FUNC" {
		return "", nil, nil, fmt.Errorf("FUNC構文が不正です: %s", line)
	}
	rest := atoms[1:]
	if len(rest) == 0 {
		return "", nil, nil, fmt.Errorf("FUNC構文が不正です(関数名がありません): %s", line)
	}
	defAtom := rest[0]
	if err := checkKind(defAtom, CatDefname); err != nil {
		return "", nil, nil, fmt.Errorf("FUNCの関数名が不正です: %w", err)
	}
	defName, err = amivmFuncNameOf(defAtom)
	if err != nil {
		return "", nil, nil, err
	}
	paramAtoms, resultAtoms, err := splitColon(rest[1:])
	if err != nil {
		return "", nil, nil, fmt.Errorf("FUNC構文が不正です: %w", err)
	}
	results, err = typeFieldsUnnamed(resultAtoms)
	if err != nil {
		return "", nil, nil, fmt.Errorf("FUNCの戻り値型が不正です: %w", err)
	}
	params, err = typeFieldsNamed(paramAtoms, func(i int) string { return amivmParamGoName(defName, i+1) })
	if err != nil {
		return "", nil, nil, fmt.Errorf("FUNCのパラメータ型が不正です: %w", err)
	}
	return defName, params, results, nil
}

// parseClosSignature は「CLOS local type1 type2 ... : type3 type4 ...」を解釈する。
// funcNameは、CLOSを内包する外側のFUNCの名前(localの解決・&Nに包まれる$N/%xxxの解決に必要)。
func parseClosSignature(line, funcName string) (localExpr ast.Expr, funcType *ast.FuncType, err error) {
	atoms := tokenizeAndClassify(line)
	if len(atoms) == 0 || atoms[0].Raw != "CLOS" {
		return nil, nil, fmt.Errorf("CLOS構文が不正です: %s", line)
	}
	rest := atoms[1:]
	if len(rest) == 0 {
		return nil, nil, fmt.Errorf("CLOS構文が不正です(代入先変数がありません): %s", line)
	}
	localAtom := rest[0]
	localExpr, err = atomExpr(localAtom, funcName, CatVa)
	if err != nil {
		return nil, nil, fmt.Errorf("CLOSの代入先が不正です: %w", err)
	}
	paramAtoms, resultAtoms, err := splitColon(rest[1:])
	if err != nil {
		return nil, nil, fmt.Errorf("CLOS構文が不正です: %w", err)
	}
	results, err := typeFieldsUnnamed(resultAtoms)
	if err != nil {
		return nil, nil, fmt.Errorf("CLOSの戻り値型が不正です: %w", err)
	}
	params, err := typeFieldsNamed(paramAtoms, func(i int) string { return closureParamGoName(i + 1) })
	if err != nil {
		return nil, nil, fmt.Errorf("CLOSのパラメータ型が不正です: %w", err)
	}
	funcType = &ast.FuncType{Params: &ast.FieldList{List: params}, Results: fieldListOrNil(results)}
	return localExpr, funcType, nil
}

// parseBody は開始位置から、blockEnd に一致する行が見つかるまでを本体としてパースする。
// SEL・CLOSは別ブロックとして先読みする。LABELはIR.txtの定義(`LABEL label → label: ;`)
// どおり常に空文とセットで1行完結する命令のため、先読みは不要で他の1行命令と同様
// parseSingleLine(defaultケース)に委ねる。
func parseBody(lines []string, start int, funcName, blockEnd string) ([]ast.Stmt, int, error) {
	var stmts []ast.Stmt
	i := start
	for i < len(lines) {
		line := lines[i]
		kw := keyword(line)

		if kw == blockEnd {
			return stmts, i + 1, nil
		}

		switch kw {
		case "":
			i++
			continue
		case "SEL":
			selStmt, next, err := parseSelectBlock(lines, i+1, funcName)
			if err != nil {
				return nil, 0, err
			}
			stmts = append(stmts, selStmt)
			i = next
			continue
		case "CLOS":
			localExpr, funcType, err := parseClosSignature(line, funcName)
			if err != nil {
				return nil, 0, err
			}
			body, next, err := parseBody(lines, i+1, funcName, "ENDCLOS")
			if err != nil {
				return nil, 0, err
			}
			funcLit := &ast.FuncLit{Type: funcType, Body: &ast.BlockStmt{List: body}}
			stmts = append(stmts, &ast.AssignStmt{
				Lhs: []ast.Expr{localExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{funcLit},
			})
			i = next
			continue
		default:
			stmt, err := parseSingleLine(line, funcName)
			if err != nil {
				return nil, 0, err
			}
			if stmt != nil {
				stmts = append(stmts, stmt)
			}
			i++
		}
	}
	return nil, 0, fmt.Errorf("対応する%sが見つかりません", blockEnd)
}

func parseSelectBlock(lines []string, start int, funcName string) (ast.Stmt, int, error) {
	var clauses []ast.Stmt
	i := start
	for i < len(lines) {
		line := lines[i]
		kw := keyword(line)
		if kw == "ENDSEL" {
			return &ast.SelectStmt{Body: &ast.BlockStmt{List: clauses}}, i + 1, nil
		}
		clause, err := parseCaseOrDefault(line, funcName)
		if err != nil {
			return nil, 0, err
		}
		clauses = append(clauses, clause)
		i++
	}
	return nil, 0, fmt.Errorf("対応するENDSELが見つかりません")
}

func parseCaseOrDefault(line, funcName string) (ast.Stmt, error) {
	atoms := tokenizeAndClassify(line)
	if len(atoms) == 0 {
		return nil, fmt.Errorf("SEL内に空行は置けません")
	}
	switch atoms[0].Raw {
	case "DEFAULT":
		if len(atoms) != 2 {
			return nil, fmt.Errorf("DEFAULT構文が不正です(書式: DEFAULT label): %s", line)
		}
		if err := checkKind(atoms[1], CatLabel); err != nil {
			return nil, fmt.Errorf("DEFAULTのラベルが不正です: %w", err)
		}
		name, err := labelGoName(atoms[1])
		if err != nil {
			return nil, err
		}
		return &ast.CommClause{Comm: nil, Body: []ast.Stmt{gotoStmt(name)}}, nil

	case "CASESEND":
		if len(atoms) != 4 {
			return nil, fmt.Errorf("CASESEND構文が不正です(書式: CASESEND single value1 label): %s", line)
		}
		ch, err := atomExpr(atoms[1], funcName, CatSingle)
		if err != nil {
			return nil, fmt.Errorf("CASESENDのチャネルが不正です: %w", err)
		}
		v, err := atomExpr(atoms[2], funcName, CatValue)
		if err != nil {
			return nil, fmt.Errorf("CASESENDの送信値が不正です: %w", err)
		}
		if err := checkKind(atoms[3], CatLabel); err != nil {
			return nil, fmt.Errorf("CASESENDのラベルが不正です: %w", err)
		}
		name, err := labelGoName(atoms[3])
		if err != nil {
			return nil, err
		}
		return &ast.CommClause{
			Comm: &ast.SendStmt{Chan: ch, Value: v},
			Body: []ast.Stmt{gotoStmt(name)},
		}, nil

	case "CASERECV":
		var destAtoms []Atom
		var chAtom Atom
		var labelAtom Atom
		switch len(atoms) {
		case 4:
			destAtoms = atoms[1:2]
			chAtom = atoms[2]
			labelAtom = atoms[3]
		case 5:
			destAtoms = atoms[1:3]
			chAtom = atoms[3]
			labelAtom = atoms[4]
		default:
			return nil, fmt.Errorf("CASERECV構文が不正です(書式: CASERECV l1 (l2) cs label): %s", line)
		}
		var lhs []ast.Expr
		for _, d := range destAtoms {
			e, err := atomExpr(d, funcName, CatMulti)
			if err != nil {
				return nil, fmt.Errorf("CASERECVの代入先が不正です: %w", err)
			}
			lhs = append(lhs, e)
		}
		ch, err := atomExpr(chAtom, funcName, CatSingle)
		if err != nil {
			return nil, fmt.Errorf("CASERECVのチャネルが不正です: %w", err)
		}
		if err := checkKind(labelAtom, CatLabel); err != nil {
			return nil, fmt.Errorf("CASERECVのラベルが不正です: %w", err)
		}
		name, err := labelGoName(labelAtom)
		if err != nil {
			return nil, err
		}
		recvStmt := &ast.AssignStmt{
			Lhs: lhs, Tok: token.ASSIGN,
			Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.ARROW, X: ch}},
		}
		return &ast.CommClause{Comm: recvStmt, Body: []ast.Stmt{gotoStmt(name)}}, nil

	default:
		return nil, fmt.Errorf("SEL内で使えるのはCASESEND/CASERECV/DEFAULTのみです: %s", line)
	}
}

// parseStructBlock は STTYPE の本体(FIELD行の並び)をENDSTTYPEまで読み取る。
func parseStructBlock(lines []string, start int) (*ast.StructType, int, error) {
	var fields []*ast.Field
	i := start
	for i < len(lines) {
		line := lines[i]
		kw := keyword(line)
		if kw == "ENDSTTYPE" {
			return &ast.StructType{Fields: &ast.FieldList{List: fields}}, i + 1, nil
		}
		if kw == "" {
			i++
			continue
		}
		if kw != "FIELD" {
			return nil, 0, fmt.Errorf("STTYPE内で使えるのはFIELDのみです: %s", line)
		}
		atoms := tokenizeAndClassify(line)
		if err := expectArgs("FIELD", atoms[1:], 2); err != nil {
			return nil, 0, err
		}
		fieldAtom, typeAtom := atoms[1], atoms[2]
		if fieldAtom.Kind != KField {
			return nil, 0, fmt.Errorf("FIELDのフィールド名が不正です: %s", fieldAtom.Raw)
		}
		typExpr, err := atomExpr(typeAtom, "", CatType)
		if err != nil {
			return nil, 0, fmt.Errorf("FIELDの型が不正です: %w", err)
		}
		fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(fieldAtom.A)}, Type: typExpr})
		i++
	}
	return nil, 0, fmt.Errorf("対応するENDSTTYPEが見つかりません")
}

// parseChType/parseSlType/parseMpType/parseFnType はトップレベルのTYPE系宣言
// (CHTYPE/SLTYPE/MPTYPE/FNTYPE)を1行から*ast.Declに変換する。

func parseChType(atoms []Atom) (ast.Decl, error) {
	if err := expectArgs("CHTYPE", atoms, 2); err != nil {
		return nil, err
	}
	if err := checkKind(atoms[0], CatDeftype); err != nil {
		return nil, fmt.Errorf("CHTYPEの型名が不正です: %w", err)
	}
	elemExpr, err := atomExpr(atoms[1], "", CatType)
	if err != nil {
		return nil, fmt.Errorf("CHTYPEの要素型が不正です: %w", err)
	}
	return typeDecl(atoms[0].A, chanTypeExpr(elemExpr)), nil
}

func parseSlType(atoms []Atom) (ast.Decl, error) {
	if err := expectArgs("SLTYPE", atoms, 2); err != nil {
		return nil, err
	}
	if err := checkKind(atoms[0], CatDeftype); err != nil {
		return nil, fmt.Errorf("SLTYPEの型名が不正です: %w", err)
	}
	elemExpr, err := atomExpr(atoms[1], "", CatType)
	if err != nil {
		return nil, fmt.Errorf("SLTYPEの要素型が不正です: %w", err)
	}
	return typeDecl(atoms[0].A, sliceTypeExpr(elemExpr)), nil
}

func parseMpType(atoms []Atom) (ast.Decl, error) {
	if err := expectArgs("MPTYPE", atoms, 3); err != nil {
		return nil, err
	}
	if err := checkKind(atoms[0], CatDeftype); err != nil {
		return nil, fmt.Errorf("MPTYPEの型名が不正です: %w", err)
	}
	keyExpr, err := atomExpr(atoms[1], "", CatType)
	if err != nil {
		return nil, fmt.Errorf("MPTYPEのキー型が不正です: %w", err)
	}
	valExpr, err := atomExpr(atoms[2], "", CatType)
	if err != nil {
		return nil, fmt.Errorf("MPTYPEの値型が不正です: %w", err)
	}
	return typeDecl(atoms[0].A, &ast.MapType{Key: keyExpr, Value: valExpr}), nil
}

func parseFnType(atoms []Atom) (ast.Decl, error) {
	if len(atoms) == 0 {
		return nil, fmt.Errorf("FNTYPE構文が不正です(型名がありません)")
	}
	deftypeAtom := atoms[0]
	if err := checkKind(deftypeAtom, CatDeftype); err != nil {
		return nil, fmt.Errorf("FNTYPEの型名が不正です: %w", err)
	}
	paramAtoms, resultAtoms, err := splitColon(atoms[1:])
	if err != nil {
		return nil, fmt.Errorf("FNTYPE構文が不正です: %w", err)
	}
	params, err := typeFieldsUnnamed(paramAtoms)
	if err != nil {
		return nil, fmt.Errorf("FNTYPEのパラメータ型が不正です: %w", err)
	}
	results, err := typeFieldsUnnamed(resultAtoms)
	if err != nil {
		return nil, fmt.Errorf("FNTYPEの戻り値型が不正です: %w", err)
	}
	ft := &ast.FuncType{Params: &ast.FieldList{List: params}, Results: fieldListOrNil(results)}
	return typeDecl(deftypeAtom.A, ft), nil
}

// =====================================================================
// プログラム全体の組み立て
// トップレベルには GVAR / FUNC / CHTYPE / SLTYPE / STTYPE / MPTYPE / FNTYPE のみ置ける
// =====================================================================

func buildProgram(source string) (*ast.File, error) {
	lines := splitLinesTrimmed(source)

	var decls []ast.Decl
	i := 0
	for i < len(lines) {
		line := lines[i]
		kw := keyword(line)
		if kw == "" {
			i++
			continue
		}

		switch kw {
		case "GVAR":
			atoms := tokenizeAndClassify(line)
			decl, err := parseGvar(atoms[1:])
			if err != nil {
				return nil, err
			}
			decls = append(decls, decl)
			i++

		case "FUNC":
			defName, params, results, err := parseFuncSignature(line)
			if err != nil {
				return nil, err
			}
			body, next, err := parseBody(lines, i+1, defName, "ENDFUNC")
			if err != nil {
				return nil, fmt.Errorf("関数 %s のパースに失敗: %w", defName, err)
			}
			funcDecl := &ast.FuncDecl{
				Name: ast.NewIdent(amivmFuncGoName(defName)),
				Type: &ast.FuncType{
					Params:  &ast.FieldList{List: params},
					Results: fieldListOrNil(results),
				},
				Body: &ast.BlockStmt{List: body},
			}
			decls = append(decls, funcDecl)
			i = next

		case "CHTYPE":
			atoms := tokenizeAndClassify(line)
			decl, err := parseChType(atoms[1:])
			if err != nil {
				return nil, err
			}
			decls = append(decls, decl)
			i++

		case "SLTYPE":
			atoms := tokenizeAndClassify(line)
			decl, err := parseSlType(atoms[1:])
			if err != nil {
				return nil, err
			}
			decls = append(decls, decl)
			i++

		case "MPTYPE":
			atoms := tokenizeAndClassify(line)
			decl, err := parseMpType(atoms[1:])
			if err != nil {
				return nil, err
			}
			decls = append(decls, decl)
			i++

		case "FNTYPE":
			atoms := tokenizeAndClassify(line)
			decl, err := parseFnType(atoms[1:])
			if err != nil {
				return nil, err
			}
			decls = append(decls, decl)
			i++

		case "STTYPE":
			atoms := tokenizeAndClassify(line)
			if err := expectArgs("STTYPE", atoms[1:], 1); err != nil {
				return nil, err
			}
			deftypeAtom := atoms[1]
			if err := checkKind(deftypeAtom, CatDeftype); err != nil {
				return nil, fmt.Errorf("STTYPEの型名が不正です: %w", err)
			}
			structType, next, err := parseStructBlock(lines, i+1)
			if err != nil {
				return nil, err
			}
			decls = append(decls, typeDecl(deftypeAtom.A, structType))
			i = next

		default:
			return nil, fmt.Errorf("トップレベルにはGVAR/FUNC/CHTYPE/SLTYPE/STTYPE/MPTYPE/FNTYPEのみ置けます。不正な行: %s", line)
		}
	}

	return &ast.File{Name: ast.NewIdent("main"), Decls: decls}, nil
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

// =====================================================================
// コンパイル・ビルドパイプライン
// =====================================================================

var (
	reUnusedNew = regexp.MustCompile(`^declared and not used:\s*(\w+)$`)
	reUnusedOld = regexp.MustCompile(`^(\w+)\s+declared but not used$`)
)

func extractUnusedVarName(msg string) (string, bool) {
	if m := reUnusedNew.FindStringSubmatch(msg); m != nil {
		return m[1], true
	}
	if m := reUnusedOld.FindStringSubmatch(msg); m != nil {
		return m[1], true
	}
	return "", false
}

func blankAssignStmt(name string) ast.Stmt {
	return &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent("_")},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{ast.NewIdent(name)},
	}
}

// declaresVarDirect は、文がVAR宣言(ast.DeclStmt, token.VAR)として指定名を直接
// (ネストしたブロックを覗かずに)宣言しているかを判定する。LABELで包まれたVAR宣言
// (*ast.LabeledStmt)も見る。
func declaresVarDirect(stmt ast.Stmt, varGoName string) bool {
	switch s := stmt.(type) {
	case *ast.DeclStmt:
		gd, ok := s.Decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			return false
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, n := range vs.Names {
				if n.Name == varGoName {
					return true
				}
			}
		}
	case *ast.LabeledStmt:
		return declaresVarDirect(s.Stmt, varGoName)
	}
	return false
}

// findAndInsertBlank は文リスト(および、IF/SELECT本体やCLOS(func literal)の内部
// といったネストしたブロック)を探索し、varGoNameのVAR宣言を見つけたらその直後に
// `_ = varGoName` を挿入する。CLOSはFUNC本体の中に入れ子のブロックスコープを作るため、
// トップレベルのfn.Body.Listだけを見る実装では見つけられない変数がある。
func findAndInsertBlank(list *[]ast.Stmt, varGoName string) bool {
	for i, stmt := range *list {
		if declaresVarDirect(stmt, varGoName) {
			newList := make([]ast.Stmt, 0, len(*list)+1)
			newList = append(newList, (*list)[:i+1]...)
			newList = append(newList, blankAssignStmt(varGoName))
			newList = append(newList, (*list)[i+1:]...)
			*list = newList
			return true
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
		if findAndInsertBlank(&s.Body.List, varGoName) {
			return true
		}
		if s.Else != nil {
			return insertBlankInNested(s.Else, varGoName)
		}
	case *ast.SelectStmt:
		for _, c := range s.Body.List {
			if cc, ok := c.(*ast.CommClause); ok {
				if findAndInsertBlank(&cc.Body, varGoName) {
					return true
				}
			}
		}
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			if fl, ok := rhs.(*ast.FuncLit); ok {
				if findAndInsertBlank(&fl.Body.List, varGoName) {
					return true
				}
			}
		}
	}
	return false
}

// insertBlankAfterDecl は、fn内でvarGoNameを宣言しているVAR文(CLOSの中など、ネストした
// ブロックスコープ内も含む)を探し、その直後に`_ = varGoName` を挿入する。関数末尾に
// 追加すると、戻り値のある関数で最後の文がreturnでなくなりmissing returnエラーを
// 誘発するため、宣言直後に挿入する。
func insertBlankAfterDecl(fn *ast.FuncDecl, varGoName string) bool {
	return findAndInsertBlank(&fn.Body.List, varGoName)
}

// appendBlankAssignsTargeted は、未使用変数が見つかった際に `_ = x` を追加する。
// 変数名は amivmLocalGoName の命名規則(<関数名>_amivm_function_<変数名>)に従っているため、
// "_amivm_function_" で分割するだけで所属関数(のGo宣言名)を特定できる。
// 該当関数・該当VAR宣言が見つからない場合のみ、安全網として全関数の末尾に追加する。
func appendBlankAssignsTargeted(f *ast.File, varNames []string) {
	for _, name := range varNames {
		parts := strings.SplitN(name, "_amivm_function_", 2)
		if len(parts) == 2 {
			funcAmivmName := parts[0]
			goFuncName := amivmFuncGoName(funcAmivmName)
			var target *ast.FuncDecl
			for _, decl := range f.Decls {
				if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == goFuncName {
					target = fd
					break
				}
			}
			if target != nil && insertBlankAfterDecl(target, name) {
				continue
			}
		}
		// フォールバック(安全網): 命名規則に一致しない、または該当VAR宣言が見つからない場合
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok {
				fd.Body.List = append(fd.Body.List, blankAssignStmt(name))
			}
		}
	}
}

func compileOnce(file *ast.File, outputPath string) ([]byte, *ast.File, []string, error) {
	fset := token.NewFileSet()
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, nil, nil, fmt.Errorf("ast整形失敗: %w", err)
	}

	resolved, err := imports.Process(outputPath, buf.Bytes(), nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("import解決失敗: %w", err)
	}

	fset2 := token.NewFileSet()
	parsedFile, err := parser.ParseFile(fset2, outputPath, resolved, parser.AllErrors)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("生成コードの構文エラー: %w", err)
	}

	var unusedVars []string
	var otherErrs []string
	conf := types.Config{
		Importer: importer.Default(),
		Error: func(err error) {
			if typeErr, ok := err.(types.Error); ok {
				if name, isUnused := extractUnusedVarName(typeErr.Msg); isUnused {
					unusedVars = append(unusedVars, name)
					return
				}
			}
			otherErrs = append(otherErrs, err.Error())
		},
	}
	conf.Check("generated", fset2, []*ast.File{parsedFile}, nil)

	if len(otherErrs) > 0 {
		return resolved, parsedFile, unusedVars, fmt.Errorf("型チェック失敗:\n%s", strings.Join(otherErrs, "\n"))
	}
	return resolved, parsedFile, unusedVars, nil
}

// generateOutput は、組み立てたast.Fileを実際のGoソースに変換し、outputPathへ書き出す。
// amivmコマンドの責務はGoソースファイルの生成までであり、go buildによる実行ファイルの
// 生成は行わない(PJ.txt/CLAUDE.mdのパイプライン図で「AMIVM → Goコード」までがamivmの
// 責務、「Goコード → 実行ファイル」は別工程として描かれていることに対応する)。
func generateOutput(file *ast.File, outputPath string, verbose bool) error {
	const maxRetries = 5
	var resolved []byte
	current := file

	for i := 0; i < maxRetries; i++ {
		var (
			unusedVars []string
			parsedFile *ast.File
			err        error
		)
		resolved, parsedFile, unusedVars, err = compileOnce(current, outputPath)
		if err != nil {
			return err
		}

		if len(unusedVars) == 0 {
			if verbose {
				fmt.Println("=== 最終生成コード ===")
				fmt.Println(string(resolved))
			}
			break
		}

		if verbose {
			fmt.Printf("未使用変数を検出したため、%v に対して `_ = x` を挿入します\n", unusedVars)
		}
		appendBlankAssignsTargeted(parsedFile, unusedVars)
		current = parsedFile

		if i == maxRetries-1 {
			return fmt.Errorf("未使用変数の解消がループ上限(%d回)に達しました", maxRetries)
		}
	}

	if err := os.WriteFile(outputPath, resolved, 0644); err != nil {
		return fmt.Errorf("書き出し失敗: %w", err)
	}

	return nil
}

// =====================================================================
// エントリポイント
// =====================================================================

const usage = "使い方: amivm <IRファイルパス> [-o <出力ファイルパス>] [-v]"

// deriveOutputPath は -o 未指定時の出力パスを決める。
// IRファイルパスの拡張子を.goに置き換える。拡張子が無ければ.goを付け足す。
func deriveOutputPath(irPath string) string {
	ext := filepath.Ext(irPath)
	if ext == "" {
		return irPath + ".go"
	}
	return strings.TrimSuffix(irPath, ext) + ".go"
}

// parseArgs はコマンドライン引数を解釈する。<IRファイルパス>・-o・-vの順序は問わない。
func parseArgs(args []string) (irPath, outPath string, verbose bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o":
			if i+1 >= len(args) {
				return "", "", false, fmt.Errorf("-oオプションには出力ファイルパスの指定が必要です")
			}
			i++
			outPath = args[i]
		case "-v":
			verbose = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", false, fmt.Errorf("不明なオプションです: %s", args[i])
			}
			if irPath != "" {
				return "", "", false, fmt.Errorf("IRファイルパスは1つだけ指定してください: %s", args[i])
			}
			irPath = args[i]
		}
	}
	if irPath == "" {
		return "", "", false, fmt.Errorf("IRファイルパスを指定してください")
	}
	return irPath, outPath, verbose, nil
}

func main() {
	irPath, outPath, verbose, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Println(err)
		fmt.Println(usage)
		os.Exit(1)
	}
	if outPath == "" {
		outPath = deriveOutputPath(irPath)
	}

	irBytes, err := os.ReadFile(irPath)
	if err != nil {
		fmt.Printf("IRファイル読み込み失敗 (%s): %v\n", irPath, err)
		os.Exit(1)
	}
	irSource := string(irBytes)

	if verbose {
		fmt.Println("=== IR ===")
		fmt.Println(irSource)
	}

	file, err := buildProgram(irSource)
	if err != nil {
		fmt.Println("IRパースエラー:", err)
		os.Exit(1)
	}

	if err := generateOutput(file, outPath, verbose); err != nil {
		fmt.Println("エラー:", err)
		os.Exit(1)
	}

	if verbose {
		fmt.Printf("生成成功: %s\n", outPath)
	}
}
