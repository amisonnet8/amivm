package main

import (
	"fmt"
	"go/ast"
	"go/token"
)

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
