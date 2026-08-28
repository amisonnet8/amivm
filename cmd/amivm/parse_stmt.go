package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

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
		return fmt.Errorf("invalid %s syntax (wrong number of arguments): %s %s", kw, kw, joinRaw(atoms))
	}
	return nil
}

// splitColon は、FNTYPE/CLOSで使う「:」1個による2分割を行う。
// コロンは戻り値・代入先と、呼び出し対象・パラメータ列を区切るために必ず1個必要。
func splitColon(atoms []Atom) (left, right []Atom, err error) {
	idx := -1
	for i, a := range atoms {
		if a.Kind == KColon {
			if idx >= 0 {
				return nil, nil, fmt.Errorf("specify exactly one colon (:): %s", joinRaw(atoms))
			}
			idx = i
		}
	}
	if idx < 0 {
		return nil, nil, fmt.Errorf("colon (:) not found: %s", joinRaw(atoms))
	}
	return atoms[:idx], atoms[idx+1:], nil
}

// splitByColons は、atoms列を「:」の個数だけセグメントに分割する(コロン自体は
// 結果に含まれない)。コロンが0個なら要素数1のセグメント列を返す。FUNC/FUNCM/
// CALL/DEFER/SPAWNのように、ジェネリクスの型引数の有無でコロンの個数が
// 1個(無し)/2個(有り)と変わる命令の可変分割に使う。
func splitByColons(atoms []Atom) [][]Atom {
	var segments [][]Atom
	start := 0
	for i, a := range atoms {
		if a.Kind == KColon {
			segments = append(segments, atoms[start:i])
			start = i + 1
		}
	}
	segments = append(segments, atoms[start:])
	return segments
}

// parseSingleLine は、ブロック開始行(FUNC/SEL/CLOS/STTYPE)・終了行・
// LABELを除いた、単一行で完結する命令をパースする。GVARはここでは扱わない
// (トップレベル専用のため、buildProgramでのみ処理される)。
func parseSingleLine(line, funcName string, closureLevel int) (ast.Stmt, error) {
	atoms := tokenizeAndClassify(line)
	if len(atoms) == 0 {
		return nil, nil
	}
	kw := atoms[0].Raw
	rest := atoms[1:]

	switch kw {
	case "VAR":
		return parseVar(rest, funcName, closureLevel)
	case "SET":
		return parseSet(rest, funcName, closureLevel)
	case "ASET":
		return parseAset(rest, funcName, closureLevel)
	case "AGET":
		return parseAget(rest, funcName, closureLevel)
	case "PSET":
		return parsePset(rest, funcName, closureLevel)
	case "PGET":
		return parsePget(rest, funcName, closureLevel)
	case "ADDR":
		return parseAddr(rest, funcName, closureLevel)
	case "ADD", "SUB", "MUL", "DIV", "MOD", "BAND", "BOR", "BXOR", "BCLEAR", "SHL", "SHR", "AND", "OR",
		"EQ", "NEQ", "LT", "LTE", "GT", "GTE":
		return parseBin2(kw, rest, funcName, closureLevel)
	case "BNOT", "NOT":
		return parseUnary(kw, rest, funcName, closureLevel)
	case "LABEL":
		return parseLabel(rest)
	case "GOTO":
		return parseGoto(rest)
	case "BREAK":
		return parseBreakOrContinue("BREAK", rest)
	case "CONTINUE":
		return parseBreakOrContinue("CONTINUE", rest)
	case "ASSERT":
		return parseAssert(rest, funcName, closureLevel)
	case "RET":
		return parseRet(rest, funcName, closureLevel)
	case "CALL":
		return parseCall(rest, funcName, closureLevel)
	case "DEFER":
		return parseDeferOrSpawn("DEFER", rest, funcName, closureLevel)
	case "SPAWN":
		return parseDeferOrSpawn("SPAWN", rest, funcName, closureLevel)
	case "CHMAKE":
		return parseChOrSlMake("CHMAKE", rest, funcName, closureLevel)
	case "SLMAKE":
		return parseChOrSlMake("SLMAKE", rest, funcName, closureLevel)
	case "MPMAKE":
		return parseMpMake(rest, funcName, closureLevel)
	case "CHSEND":
		return parseChSend(rest, funcName, closureLevel)
	case "CHRECV":
		return parseChRecv(rest, funcName, closureLevel)
	case "CONCAT":
		return parseConcat(rest, funcName, closureLevel)
	case "SLICE":
		return parseSlice(rest, funcName, closureLevel)
	case "FSET":
		return parseFset(rest, funcName, closureLevel)
	case "FGET":
		return parseFget(rest, funcName, closureLevel)
	case "METHVAL":
		return parseMethval(rest, funcName, closureLevel)
	case "FUNCVAL":
		return parseFuncval(rest, funcName, closureLevel)
	case "MSET":
		return parseMset(rest, funcName, closureLevel)
	case "MGET":
		return parseMget(rest, funcName, closureLevel)
	case "MPKEYS":
		return parseMpKeys(rest, funcName, closureLevel)
	default:
		return nil, fmt.Errorf("unknown instruction: %s", line)
	}
}

func parseVar(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("VAR", atoms, 2); err != nil {
		return nil, err
	}
	if funcName == "" {
		return nil, fmt.Errorf("VAR can only be used inside a function (use GVAR at the top level)")
	}
	if err := checkKind(atoms[0], CatVa); err != nil {
		return nil, fmt.Errorf("invalid VAR variable name: %w", err)
	}
	typExpr, err := atomExpr(atoms[1], "", 0, CatType)
	if err != nil {
		return nil, fmt.Errorf("invalid VAR type: %w", err)
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
		return nil, fmt.Errorf("invalid GVAR variable name: %w", err)
	}
	typExpr, err := atomExpr(atoms[1], "", 0, CatType)
	if err != nil {
		return nil, fmt.Errorf("invalid GVAR type: %w", err)
	}
	return &ast.GenDecl{
		Tok: token.VAR,
		Specs: []ast.Spec{
			&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(atoms[0].A)}, Type: typExpr},
		},
	}, nil
}

func parseSet(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("SET", atoms, 2); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid SET left-hand side: %w", err)
	}
	rhs, err := atomExpr(atoms[1], funcName, closureLevel, CatValue)
	if err != nil {
		return nil, fmt.Errorf("invalid SET right-hand side: %w", err)
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{rhs}}, nil
}

func parseAset(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("ASET", atoms, 3); err != nil {
		return nil, err
	}
	arr, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid ASET array: %w", err)
	}
	idx, err := atomExpr(atoms[1], funcName, closureLevel, CatWhole)
	if err != nil {
		return nil, fmt.Errorf("invalid ASET index: %w", err)
	}
	val, err := atomExpr(atoms[2], funcName, closureLevel, CatValue)
	if err != nil {
		return nil, fmt.Errorf("invalid ASET value: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.IndexExpr{X: arr, Index: idx}}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{val},
	}, nil
}

func parseAget(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("AGET", atoms, 3); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid AGET assignment target: %w", err)
	}
	variable, err := atomExpr(atoms[1], funcName, closureLevel, CatVariable)
	if err != nil {
		return nil, fmt.Errorf("invalid AGET array: %w", err)
	}
	idx, err := atomExpr(atoms[2], funcName, closureLevel, CatWhole)
	if err != nil {
		return nil, fmt.Errorf("invalid AGET index: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.IndexExpr{X: variable, Index: idx}},
	}, nil
}

func parsePset(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("PSET", atoms, 2); err != nil {
		return nil, err
	}
	ptr, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid PSET pointer: %w", err)
	}
	val, err := atomExpr(atoms[1], funcName, closureLevel, CatValue)
	if err != nil {
		return nil, fmt.Errorf("invalid PSET value: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.StarExpr{X: ptr}}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{val},
	}, nil
}

func parsePget(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("PGET", atoms, 2); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid PGET assignment target: %w", err)
	}
	variable, err := atomExpr(atoms[1], funcName, closureLevel, CatVariable)
	if err != nil {
		return nil, fmt.Errorf("invalid PGET pointer: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.StarExpr{X: variable}},
	}, nil
}

// parseAddr は「ADDR single variable (point)」を解釈する。pointは省略可能。
// pointが無ければ single = &variable、pointが>xxx_123(構造体フィールド)なら
// single = &variable.point、それ以外(添字)なら single = &variable[point] を生成する。
// &variable[point]はスライス/配列専用で、mapに使うとgo/types側でエラーになる
// (AMIVM側では検証しない。意味の正しさをgo/typesに委ねる設計方針どおり)。
func parseAddr(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if len(atoms) < 2 || len(atoms) > 3 {
		return nil, fmt.Errorf("invalid ADDR syntax (format: ADDR single variable (point)): %s", joinRaw(atoms))
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid ADDR assignment target: %w", err)
	}
	variable, err := atomExpr(atoms[1], funcName, closureLevel, CatVariable)
	if err != nil {
		return nil, fmt.Errorf("invalid ADDR target: %w", err)
	}

	var target ast.Expr = variable
	if len(atoms) == 3 {
		pointAtom := atoms[2]
		if err := checkKind(pointAtom, CatPoint); err != nil {
			return nil, fmt.Errorf("invalid ADDR point: %w", err)
		}
		if pointAtom.Kind == KField {
			target = &ast.SelectorExpr{X: variable, Sel: ast.NewIdent(pointAtom.A)}
		} else {
			point, err := atomToExpr(pointAtom, funcName, closureLevel)
			if err != nil {
				return nil, fmt.Errorf("invalid ADDR point: %w", err)
			}
			target = &ast.IndexExpr{X: variable, Index: point}
		}
	}

	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: target}},
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

func parseBin2(kw string, atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs(kw, atoms, 3); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid %s left-hand side: %w", kw, err)
	}
	cats := binOperandCategories[kw]
	a, err := atomExpr(atoms[1], funcName, closureLevel, cats[0])
	if err != nil {
		return nil, fmt.Errorf("invalid %s first operand: %w", kw, err)
	}
	b, err := atomExpr(atoms[2], funcName, closureLevel, cats[1])
	if err != nil {
		return nil, fmt.Errorf("invalid %s second operand: %w", kw, err)
	}
	op := binOpTokens[kw]
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.BinaryExpr{X: a, Op: op, Y: b}},
	}, nil
}

func parseUnary(kw string, atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs(kw, atoms, 2); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid %s left-hand side: %w", kw, err)
	}
	cat := CatBool
	op := token.NOT
	if kw == "BNOT" {
		cat = CatInt
		op = token.XOR
	}
	a, err := atomExpr(atoms[1], funcName, closureLevel, cat)
	if err != nil {
		return nil, fmt.Errorf("invalid %s operand: %w", kw, err)
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
		return nil, fmt.Errorf("invalid LABEL name: %w", err)
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
		return nil, fmt.Errorf("invalid GOTO label: %w", err)
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

// parseBreakOrContinue は「BREAK」「CONTINUE」を解釈する(どちらもオペランド無し)。
func parseBreakOrContinue(kw string, atoms []Atom) (ast.Stmt, error) {
	if err := expectArgs(kw, atoms, 0); err != nil {
		return nil, err
	}
	tok := token.BREAK
	if kw == "CONTINUE" {
		tok = token.CONTINUE
	}
	return &ast.BranchStmt{Tok: tok}, nil
}

func parseRet(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	var results []ast.Expr
	for _, a := range atoms {
		v, err := atomExpr(a, funcName, closureLevel, CatValue)
		if err != nil {
			return nil, fmt.Errorf("invalid RET return value: %w", err)
		}
		results = append(results, v)
	}
	return &ast.ReturnStmt{Results: results}, nil
}

// typeExprList はatoms列(いずれもtypeカテゴリ)を[]ast.Exprに変換する。
// GETYPEの型引数、CALL/DEFER/SPAWNの明示的型引数で使う。
func typeExprList(atoms []Atom) ([]ast.Expr, error) {
	var exprs []ast.Expr
	for _, a := range atoms {
		e, err := atomExpr(a, "", 0, CatType)
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, e)
	}
	return exprs, nil
}

// buildCallExpr は呼び出し対象・(あれば)明示的型引数・引数列からast.CallExprを
// 組み立てる。typeArgAtomsが空ならinstantiateTypeExprはfnをそのまま返す
// (非ジェネリクス呼び出し)。
func buildCallExpr(callAtom Atom, typeArgAtoms []Atom, argAtoms []Atom, funcName string, closureLevel int) (*ast.CallExpr, error) {
	fn, err := atomExpr(callAtom, funcName, closureLevel, CatCallname)
	if err != nil {
		return nil, fmt.Errorf("invalid call target: %w", err)
	}
	typeArgs, err := typeExprList(typeArgAtoms)
	if err != nil {
		return nil, fmt.Errorf("invalid type argument: %w", err)
	}
	fn = instantiateTypeExpr(fn, typeArgs)
	var args []ast.Expr
	for _, a := range argAtoms {
		v, err := atomExpr(a, funcName, closureLevel, CatValue)
		if err != nil {
			return nil, fmt.Errorf("invalid argument: %w", err)
		}
		args = append(args, v)
	}
	return &ast.CallExpr{Fun: fn, Args: args}, nil
}

// parseCall は「CALL multi1 multi2 ... : callname (type1 type2 ... :) value1 value2 ...」
// を解釈する。最初のコロンの左側(多重代入先)は空でもよい(その場合は式文として
// 呼び出すだけになる)。コロンがもう1個あれば、callnameの直後は明示的型引数になる。
func parseCall(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	segments := splitByColons(atoms)
	var destAtoms, calleeAtoms, argAtoms []Atom
	switch len(segments) {
	case 2:
		destAtoms, calleeAtoms = segments[0], segments[1]
	case 3:
		destAtoms, calleeAtoms, argAtoms = segments[0], segments[1], segments[2]
	default:
		return nil, fmt.Errorf("invalid CALL syntax: %s", joinRaw(atoms))
	}
	if len(calleeAtoms) == 0 {
		return nil, fmt.Errorf("invalid CALL syntax (missing call target): %s", joinRaw(atoms))
	}
	callAtom, typeArgAtoms := calleeAtoms[0], calleeAtoms[1:]
	if len(segments) == 2 {
		// コロン1個: calleeAtomsの残りがそのまま値引数(型引数なし)
		argAtoms = typeArgAtoms
		typeArgAtoms = nil
	}

	callExpr, err := buildCallExpr(callAtom, typeArgAtoms, argAtoms, funcName, closureLevel)
	if err != nil {
		return nil, err
	}

	if len(destAtoms) == 0 {
		return &ast.ExprStmt{X: callExpr}, nil
	}

	var lhs []ast.Expr
	for _, d := range destAtoms {
		e, err := atomExpr(d, funcName, closureLevel, CatMulti)
		if err != nil {
			return nil, fmt.Errorf("invalid CALL assignment target: %w", err)
		}
		lhs = append(lhs, e)
	}
	return &ast.AssignStmt{Lhs: lhs, Tok: token.ASSIGN, Rhs: []ast.Expr{callExpr}}, nil
}

// parseDeferOrSpawn は「DEFER/SPAWN callname (type1 type2 ... :) value1 value2 ...」を
// 解釈する。コロンが無ければ型引数無し、コロンが1個あればcallname直後が型引数になる。
func parseDeferOrSpawn(kw string, atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if len(atoms) == 0 {
		return nil, fmt.Errorf("invalid %s syntax (missing call target): %s", kw, kw)
	}
	segments := splitByColons(atoms)
	var calleeAtoms, typeArgAtoms, argAtoms []Atom
	switch len(segments) {
	case 1:
		calleeAtoms = segments[0]
	case 2:
		calleeAtoms, argAtoms = segments[0], segments[1]
		typeArgAtoms = calleeAtoms[1:]
	default:
		return nil, fmt.Errorf("invalid %s syntax: %s", kw, joinRaw(atoms))
	}
	if len(calleeAtoms) == 0 {
		return nil, fmt.Errorf("invalid %s syntax (missing call target): %s", kw, joinRaw(atoms))
	}
	callAtom := calleeAtoms[0]
	if len(segments) == 1 {
		argAtoms = calleeAtoms[1:]
	}
	callExpr, err := buildCallExpr(callAtom, typeArgAtoms, argAtoms, funcName, closureLevel)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", kw, err)
	}
	if kw == "DEFER" {
		return &ast.DeferStmt{Call: callExpr}, nil
	}
	return &ast.GoStmt{Call: callExpr}, nil
}

func parseChOrSlMake(kw string, atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs(kw, atoms, 3); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid %s variable: %w", kw, err)
	}
	typExpr, err := atomExpr(atoms[1], "", 0, CatTypename)
	if err != nil {
		return nil, fmt.Errorf("invalid %s type: %w", kw, err)
	}
	size, err := atomExpr(atoms[2], funcName, closureLevel, CatWhole)
	if err != nil {
		return nil, fmt.Errorf("invalid %s size: %w", kw, err)
	}
	makeCall := &ast.CallExpr{Fun: ast.NewIdent("make"), Args: []ast.Expr{typExpr, size}}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{makeCall},
	}, nil
}

func parseMpMake(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("MPMAKE", atoms, 2); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid MPMAKE variable: %w", err)
	}
	typExpr, err := atomExpr(atoms[1], "", 0, CatTypename)
	if err != nil {
		return nil, fmt.Errorf("invalid MPMAKE type: %w", err)
	}
	makeCall := &ast.CallExpr{Fun: ast.NewIdent("make"), Args: []ast.Expr{typExpr}}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{makeCall},
	}, nil
}

func parseChSend(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("CHSEND", atoms, 2); err != nil {
		return nil, err
	}
	ch, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid CHSEND channel: %w", err)
	}
	v, err := atomExpr(atoms[1], funcName, closureLevel, CatValue)
	if err != nil {
		return nil, fmt.Errorf("invalid CHSEND send value: %w", err)
	}
	return &ast.SendStmt{Chan: ch, Value: v}, nil
}

func parseChRecv(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if len(atoms) < 2 || len(atoms) > 3 {
		return nil, fmt.Errorf("invalid CHRECV syntax (format: CHRECV l1 (l2) cs): %s", joinRaw(atoms))
	}
	destAtoms := atoms[:len(atoms)-1]
	chAtom := atoms[len(atoms)-1]

	var lhs []ast.Expr
	for _, d := range destAtoms {
		e, err := atomExpr(d, funcName, closureLevel, CatMulti)
		if err != nil {
			return nil, fmt.Errorf("invalid CHRECV assignment target: %w", err)
		}
		lhs = append(lhs, e)
	}
	ch, err := atomExpr(chAtom, funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid CHRECV channel: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: lhs, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.ARROW, X: ch}},
	}, nil
}

func parseConcat(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if len(atoms) < 3 {
		return nil, fmt.Errorf("invalid CONCAT syntax (format: CONCAT l1 s1 s2 ...): %s", joinRaw(atoms))
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid CONCAT left-hand side: %w", err)
	}
	var parts []ast.Expr
	for _, a := range atoms[1:] {
		v, err := atomExpr(a, funcName, closureLevel, CatSlice)
		if err != nil {
			return nil, fmt.Errorf("invalid CONCAT operand: %w", err)
		}
		parts = append(parts, v)
	}
	expr := parts[0]
	for _, p := range parts[1:] {
		expr = &ast.BinaryExpr{X: expr, Op: token.ADD, Y: p}
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{expr}}, nil
}

func parseSlice(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("SLICE", atoms, 4); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid SLICE assignment target: %w", err)
	}
	src, err := atomExpr(atoms[1], funcName, closureLevel, CatSlice)
	if err != nil {
		return nil, fmt.Errorf("invalid SLICE target: %w", err)
	}
	var low, high ast.Expr
	if _, err := atomExpr(atoms[2], funcName, closureLevel, CatFromTo); err != nil {
		return nil, fmt.Errorf("invalid SLICE from: %w", err)
	}
	if atoms[2].Kind != KBlank {
		low, _ = atomToExpr(atoms[2], funcName, closureLevel)
	}
	if _, err := atomExpr(atoms[3], funcName, closureLevel, CatFromTo); err != nil {
		return nil, fmt.Errorf("invalid SLICE to: %w", err)
	}
	if atoms[3].Kind != KBlank {
		high, _ = atomToExpr(atoms[3], funcName, closureLevel)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.SliceExpr{X: src, Low: low, High: high}},
	}, nil
}

func parseFset(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("FSET", atoms, 3); err != nil {
		return nil, err
	}
	obj, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid FSET target: %w", err)
	}
	if atoms[1].Kind != KField {
		return nil, fmt.Errorf("invalid FSET field name: %s", atoms[1].Raw)
	}
	val, err := atomExpr(atoms[2], funcName, closureLevel, CatValue)
	if err != nil {
		return nil, fmt.Errorf("invalid FSET value: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.SelectorExpr{X: obj, Sel: ast.NewIdent(atoms[1].A)}}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{val},
	}, nil
}

func parseFget(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("FGET", atoms, 3); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid FGET assignment target: %w", err)
	}
	variable, err := atomExpr(atoms[1], funcName, closureLevel, CatVariable)
	if err != nil {
		return nil, fmt.Errorf("invalid FGET target: %w", err)
	}
	if atoms[2].Kind != KField {
		return nil, fmt.Errorf("invalid FGET field name: %s", atoms[2].Raw)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.SelectorExpr{X: variable, Sel: ast.NewIdent(atoms[2].A)}},
	}, nil
}

// parseMethval は「METHVAL local variable method」を解釈する
// (local := variable.method)。localはVARで事前宣言しない。FNTYPEで宣言した
// 関数型とGoの実際のメソッド値の型が(概念上は同じでも)完全一致せず代入できない
// ケースがあるため、:=によるGoの型推論に任せることでこれを回避する。localが
// CatVa(%xxx_123のみ)なのはこの:=の意味論と整合させるため($N/&N/@xxxのような
// 既存の宣言済み識別子は:=の左辺に使えない)。
func parseMethval(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("METHVAL", atoms, 3); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatVa)
	if err != nil {
		return nil, fmt.Errorf("invalid METHVAL assignment target: %w", err)
	}
	variable, err := atomExpr(atoms[1], funcName, closureLevel, CatVariable)
	if err != nil {
		return nil, fmt.Errorf("invalid METHVAL target: %w", err)
	}
	if atoms[2].Kind != KMethod {
		return nil, fmt.Errorf("invalid METHVAL method name: %s", atoms[2].Raw)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.SelectorExpr{X: variable, Sel: ast.NewIdent(atoms[2].A)}},
	}, nil
}

// parseFuncval は「FUNCVAL local callname」を解釈する(local := callname)。
// レシーバーを持たない関数値(Goライブラリのパッケージ関数・AMIVM関数・既存の
// 関数値変数)を値として取り出す、METHVALの兄弟命令。callnameはCALLと同じ
// カテゴリ(!xxx/?xxx/?xxx.yyy/%xxx/@xxx/$N/&N/&L-N)を使う。localはMETHVALと
// 同じ理由でVARで事前宣言しない(CatVa=%xxx_123のみ)。
func parseFuncval(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("FUNCVAL", atoms, 2); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatVa)
	if err != nil {
		return nil, fmt.Errorf("invalid FUNCVAL assignment target: %w", err)
	}
	rhs, err := atomExpr(atoms[1], funcName, closureLevel, CatCallname)
	if err != nil {
		return nil, fmt.Errorf("invalid FUNCVAL target: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.DEFINE,
		Rhs: []ast.Expr{rhs},
	}, nil
}

func parseMset(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("MSET", atoms, 3); err != nil {
		return nil, err
	}
	m, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid MSET map: %w", err)
	}
	key, err := atomExpr(atoms[1], funcName, closureLevel, CatValue)
	if err != nil {
		return nil, fmt.Errorf("invalid MSET key: %w", err)
	}
	val, err := atomExpr(atoms[2], funcName, closureLevel, CatValue)
	if err != nil {
		return nil, fmt.Errorf("invalid MSET value: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.IndexExpr{X: m, Index: key}}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{val},
	}, nil
}

func parseMget(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if len(atoms) < 3 || len(atoms) > 4 {
		return nil, fmt.Errorf("invalid MGET syntax (format: MGET l1 (l2) m v1): %s", joinRaw(atoms))
	}
	valueAtom := atoms[len(atoms)-1]
	mapAtom := atoms[len(atoms)-2]
	destAtoms := atoms[:len(atoms)-2]

	var lhs []ast.Expr
	for _, d := range destAtoms {
		e, err := atomExpr(d, funcName, closureLevel, CatMulti)
		if err != nil {
			return nil, fmt.Errorf("invalid MGET assignment target: %w", err)
		}
		lhs = append(lhs, e)
	}
	m, err := atomExpr(mapAtom, funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid MGET map: %w", err)
	}
	key, err := atomExpr(valueAtom, funcName, closureLevel, CatValue)
	if err != nil {
		return nil, fmt.Errorf("invalid MGET key: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: lhs, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.IndexExpr{X: m, Index: key}},
	}, nil
}

// parseMpKeys は「MPKEYS single1 single2」を解釈する。mapを走査する手段が無いため、
// single1 = slices.Collect(maps.Keys(single2)) というGoコードを生成する
// (slices/maps標準パッケージ。importはgoimportsが自動解決する)。
func parseMpKeys(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if err := expectArgs("MPKEYS", atoms, 2); err != nil {
		return nil, err
	}
	lhs, err := atomExpr(atoms[0], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid MPKEYS assignment target: %w", err)
	}
	m, err := atomExpr(atoms[1], funcName, closureLevel, CatSingle)
	if err != nil {
		return nil, fmt.Errorf("invalid MPKEYS map: %w", err)
	}
	keysCall := &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent("maps"), Sel: ast.NewIdent("Keys")},
		Args: []ast.Expr{m},
	}
	collectCall := &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent("slices"), Sel: ast.NewIdent("Collect")},
		Args: []ast.Expr{keysCall},
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN,
		Rhs: []ast.Expr{collectCall},
	}, nil
}

// parseAssert は「ASSERT multi1 (multi2) variable type1」を解釈する
// (multi1(, multi2) = variable.(type1))。multi2省略時は失敗するとpanicする単一形、
// 指定時は2つ目がok値になり失敗してもpanicしない(Goの型アサーションと同じ)。
func parseAssert(atoms []Atom, funcName string, closureLevel int) (ast.Stmt, error) {
	if len(atoms) < 3 || len(atoms) > 4 {
		return nil, fmt.Errorf("invalid ASSERT syntax (format: ASSERT l1 (l2) variable type1): %s", joinRaw(atoms))
	}
	typeAtom := atoms[len(atoms)-1]
	variableAtom := atoms[len(atoms)-2]
	destAtoms := atoms[:len(atoms)-2]

	var lhs []ast.Expr
	for _, d := range destAtoms {
		e, err := atomExpr(d, funcName, closureLevel, CatMulti)
		if err != nil {
			return nil, fmt.Errorf("invalid ASSERT assignment target: %w", err)
		}
		lhs = append(lhs, e)
	}
	variable, err := atomExpr(variableAtom, funcName, closureLevel, CatVariable)
	if err != nil {
		return nil, fmt.Errorf("invalid ASSERT target: %w", err)
	}
	typExpr, err := atomExpr(typeAtom, "", 0, CatType)
	if err != nil {
		return nil, fmt.Errorf("invalid ASSERT type: %w", err)
	}
	return &ast.AssignStmt{
		Lhs: lhs, Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.TypeAssertExpr{X: variable, Type: typExpr}},
	}, nil
}
