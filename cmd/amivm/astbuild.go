package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
)

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
// (&L-N → amivm_closureL_paramN。Lはこのクロージャーのネスト深さで、func literalごとに
// 別スコープになるため関数名を含めなくても衝突しない)。
func closureParamGoName(level, n int) string {
	return fmt.Sprintf("amivm_closure%d_param%d", level, n)
}

func paramBaseExpr(funcName, numStr string) (ast.Expr, error) {
	if funcName == "" {
		return nil, fmt.Errorf("$%sは関数定義の外では使えません", numStr)
	}
	n, _ := strconv.Atoi(numStr)
	return ast.NewIdent(amivmParamGoName(funcName, n)), nil
}

// closureParamBaseExpr は &N(自分がいるCLOS階層のN番目)/&L-N(階層Lを明示)を解決する。
// aのBが空なら現在の階層(closureLevel)、そうでなければBをパースした値を階層として使う。
func closureParamBaseExpr(a Atom, closureLevel int) ast.Expr {
	n, _ := strconv.Atoi(a.A)
	level := closureLevel
	if a.B != "" {
		level, _ = strconv.Atoi(a.B)
	}
	return ast.NewIdent(closureParamGoName(level, n))
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

func atomToExpr(a Atom, funcName string, closureLevel int) (ast.Expr, error) {
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
		return closureParamBaseExpr(a, closureLevel), nil
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
