package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strings"
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
		te, err := atomExpr(a, "", 0, CatType)
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
		te, err := atomExpr(a, "", 0, CatType)
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

// parseClosSignature は「CLOS single1 type1 type2 ... : type3 type4 ...」を解釈する。
// funcNameは、CLOSを内包する外側のFUNCの名前(代入先の解決・&Nに包まれる$N/%xxxの解決に必要)。
// currentLevelは、このCLOS命令が書かれている位置自体のクロージャーネスト深さ(FUNC直下なら0)。
// 代入先(single1)は現在のスコープにある変数を指すためcurrentLevelで解決するが、これから
// 作る本体はさらに1段ネストが深くなるため、そのnewLevel(=currentLevel+1)をパラメータの
// 命名・戻り値として使う。
func parseClosSignature(line, funcName string, currentLevel int) (targetExpr ast.Expr, funcType *ast.FuncType, newLevel int, err error) {
	atoms := tokenizeAndClassify(line)
	if len(atoms) == 0 || atoms[0].Raw != "CLOS" {
		return nil, nil, 0, fmt.Errorf("CLOS構文が不正です: %s", line)
	}
	rest := atoms[1:]
	if len(rest) == 0 {
		return nil, nil, 0, fmt.Errorf("CLOS構文が不正です(代入先変数がありません): %s", line)
	}
	targetAtom := rest[0]
	targetExpr, err = atomExpr(targetAtom, funcName, currentLevel, CatSingle)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("CLOSの代入先が不正です: %w", err)
	}
	paramAtoms, resultAtoms, err := splitColon(rest[1:])
	if err != nil {
		return nil, nil, 0, fmt.Errorf("CLOS構文が不正です: %w", err)
	}
	results, err := typeFieldsUnnamed(resultAtoms)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("CLOSの戻り値型が不正です: %w", err)
	}
	newLevel = currentLevel + 1
	params, err := typeFieldsNamed(paramAtoms, func(i int) string { return closureParamGoName(newLevel, i+1) })
	if err != nil {
		return nil, nil, 0, fmt.Errorf("CLOSのパラメータ型が不正です: %w", err)
	}
	funcType = &ast.FuncType{Params: &ast.FieldList{List: params}, Results: fieldListOrNil(results)}
	return targetExpr, funcType, newLevel, nil
}

// parseBody は開始位置から、blockEnds のいずれかに一致する行が見つかるまでを本体として
// パースする。SEL・CLOS・IF・LOOPは別ブロックとして先読みする。LABELはIR.txtの定義
// (`LABEL label → label: ;`)どおり常に空文とセットで1行完結する命令のため、先読みは
// 不要で他の1行命令と同様parseSingleLine(defaultケース)に委ねる。
//
// blockEndsは1個(FUNC本体なら"ENDFUNC"、CLOS本体なら"ENDCLOS"、LOOP本体なら"ENDLOOP"、
// SELの各ケース本体なら"CASESEND"/"CASERECV"/"DEFAULT"/"ENDSEL"の4個)を渡す。IFの各
// 分岐本体は"ELIF"/"ELSE"/"ENDIF"の3個、ELSE本体だけは"ENDIF"の1個。戻り値のmatchedは、
// 実際にどのblockEndで止まったかを呼び出し側(IF/SELの分岐)に伝えるためのもの。
func parseBody(lines []string, start int, funcName string, closureLevel int, blockEnds []string) (stmts []ast.Stmt, next int, matched string, err error) {
	i := start
	for i < len(lines) {
		line := lines[i]
		kw := keyword(line)

		if slices.Contains(blockEnds, kw) {
			return stmts, i + 1, kw, nil
		}

		switch kw {
		case "":
			i++
			continue
		case "SEL":
			selStmt, next, err := parseSelectBlock(lines, i+1, funcName, closureLevel)
			if err != nil {
				return nil, 0, "", err
			}
			stmts = append(stmts, selStmt)
			i = next
			continue
		case "CLOS":
			targetExpr, funcType, newLevel, err := parseClosSignature(line, funcName, closureLevel)
			if err != nil {
				return nil, 0, "", err
			}
			body, next, _, err := parseBody(lines, i+1, funcName, newLevel, []string{"ENDCLOS"})
			if err != nil {
				return nil, 0, "", err
			}
			funcLit := &ast.FuncLit{Type: funcType, Body: &ast.BlockStmt{List: body}}
			stmts = append(stmts, &ast.AssignStmt{
				Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{funcLit},
			})
			i = next
			continue
		case "IF":
			ifStmt, next, err := parseIfChain(lines, i, funcName, closureLevel)
			if err != nil {
				return nil, 0, "", err
			}
			stmts = append(stmts, ifStmt)
			i = next
			continue
		case "LOOP":
			loopStmt, next, err := parseLoopBlock(lines, i, funcName, closureLevel)
			if err != nil {
				return nil, 0, "", err
			}
			stmts = append(stmts, loopStmt)
			i = next
			continue
		case "ELIF", "ELSE":
			return nil, 0, "", fmt.Errorf("%sはIFの外、またはELSEの後には使えません: %s", kw, line)
		case "ENDIF":
			return nil, 0, "", fmt.Errorf("対応するIFが見つかりません: %s", line)
		case "ENDLOOP":
			return nil, 0, "", fmt.Errorf("対応するLOOPが見つかりません: %s", line)
		default:
			stmt, err := parseSingleLine(line, funcName, closureLevel)
			if err != nil {
				return nil, 0, "", err
			}
			if stmt != nil {
				stmts = append(stmts, stmt)
			}
			i++
		}
	}
	return nil, 0, "", fmt.Errorf("対応する%sが見つかりません", strings.Join(blockEnds, "または"))
}

// parseIfChain は「IF boolean1」(または再帰呼び出しでは「ELIF boolean1」)から始まる
// 分岐チェーンをENDIFまで解析する。headerIdxはそのIF/ELIF行自体のインデックス。
// ELIFが見つかれば同じ関数を再帰し、それをast.IfStmt.Elseへ、ELSEが見つかれば
// その本体(ENDIFまで)をast.IfStmt.Elseへ*ast.BlockStmtとして据える。ELSEの後に
// さらにELIF/ELSEが続く場合は、ELSE本体をblockEnds=["ENDIF"]だけで読むため、
// parseBodyの「ELIF/ELSEは予約語」ケースがそれを拾って構文エラーにする。
func parseIfChain(lines []string, headerIdx int, funcName string, closureLevel int) (ast.Stmt, int, error) {
	atoms := tokenizeAndClassify(lines[headerIdx])
	if len(atoms) != 2 || (atoms[0].Raw != "IF" && atoms[0].Raw != "ELIF") {
		return nil, 0, fmt.Errorf("IF/ELIF構文が不正です(書式: IF boolean1 / ELIF boolean1): %s", lines[headerIdx])
	}
	kw := atoms[0].Raw
	cond, err := atomExpr(atoms[1], funcName, closureLevel, CatBool)
	if err != nil {
		return nil, 0, fmt.Errorf("%sの条件が不正です: %w", kw, err)
	}

	body, next, matched, err := parseBody(lines, headerIdx+1, funcName, closureLevel, []string{"ELIF", "ELSE", "ENDIF"})
	if err != nil {
		return nil, 0, err
	}

	ifStmt := &ast.IfStmt{Cond: cond, Body: &ast.BlockStmt{List: body}}

	switch matched {
	case "ENDIF":
		return ifStmt, next, nil
	case "ELIF":
		elseStmt, next2, err := parseIfChain(lines, next-1, funcName, closureLevel)
		if err != nil {
			return nil, 0, err
		}
		ifStmt.Else = elseStmt
		return ifStmt, next2, nil
	default: // "ELSE"
		elseBody, next2, _, err := parseBody(lines, next, funcName, closureLevel, []string{"ENDIF"})
		if err != nil {
			return nil, 0, err
		}
		ifStmt.Else = &ast.BlockStmt{List: elseBody}
		return ifStmt, next2, nil
	}
}

// parseLoopBlock は「LOOP」からENDLOOPまでを解析する。loopIdxはLOOP行自体のインデックス。
func parseLoopBlock(lines []string, loopIdx int, funcName string, closureLevel int) (ast.Stmt, int, error) {
	atoms := tokenizeAndClassify(lines[loopIdx])
	if len(atoms) != 1 || atoms[0].Raw != "LOOP" {
		return nil, 0, fmt.Errorf("LOOP構文が不正です(書式: LOOP): %s", lines[loopIdx])
	}
	body, next, _, err := parseBody(lines, loopIdx+1, funcName, closureLevel, []string{"ENDLOOP"})
	if err != nil {
		return nil, 0, err
	}
	return &ast.ForStmt{Body: &ast.BlockStmt{List: body}}, next, nil
}

// parseSelectBlock は「SEL」からENDSELまでを解析する。startはSEL直後の行のインデックス。
// CASESEND/CASERECV/DEFAULTはもはやlabelを取らず、次のCASESEND/CASERECV/DEFAULTか
// ENDSELまでを自分の本体として持つブロックになっている(旧仕様は単一行のgotoのみ)。
func parseSelectBlock(lines []string, start int, funcName string, closureLevel int) (ast.Stmt, int, error) {
	caseKeywords := []string{"CASESEND", "CASERECV", "DEFAULT", "ENDSEL"}
	var clauses []ast.Stmt
	i := start
	for {
		if i >= len(lines) {
			return nil, 0, fmt.Errorf("対応するENDSELが見つかりません")
		}
		kw := keyword(lines[i])
		if kw == "ENDSEL" {
			return &ast.SelectStmt{Body: &ast.BlockStmt{List: clauses}}, i + 1, nil
		}
		comm, err := parseCaseHeader(lines[i], funcName, closureLevel)
		if err != nil {
			return nil, 0, err
		}
		body, next, _, err := parseBody(lines, i+1, funcName, closureLevel, caseKeywords)
		if err != nil {
			return nil, 0, err
		}
		clauses = append(clauses, &ast.CommClause{Comm: comm, Body: body})
		i = next - 1
	}
}

// parseCaseHeader は「CASESEND single1 value1」/「CASERECV multi1 (multi2) single1」/
// 「DEFAULT」のヘッダ行を解釈し、ast.CommClause.Commに使うast.Stmtを返す
// (DEFAULTはnil)。本体側はparseSelectBlockがparseBodyへ委譲する。
func parseCaseHeader(line, funcName string, closureLevel int) (ast.Stmt, error) {
	atoms := tokenizeAndClassify(line)
	if len(atoms) == 0 {
		return nil, fmt.Errorf("SEL内に空行は置けません")
	}
	switch atoms[0].Raw {
	case "DEFAULT":
		if err := expectArgs("DEFAULT", atoms[1:], 0); err != nil {
			return nil, err
		}
		return nil, nil

	case "CASESEND":
		if err := expectArgs("CASESEND", atoms[1:], 2); err != nil {
			return nil, err
		}
		ch, err := atomExpr(atoms[1], funcName, closureLevel, CatSingle)
		if err != nil {
			return nil, fmt.Errorf("CASESENDのチャネルが不正です: %w", err)
		}
		v, err := atomExpr(atoms[2], funcName, closureLevel, CatValue)
		if err != nil {
			return nil, fmt.Errorf("CASESENDの送信値が不正です: %w", err)
		}
		return &ast.SendStmt{Chan: ch, Value: v}, nil

	case "CASERECV":
		var destAtoms []Atom
		var chAtom Atom
		switch len(atoms) {
		case 3:
			destAtoms = atoms[1:2]
			chAtom = atoms[2]
		case 4:
			destAtoms = atoms[1:3]
			chAtom = atoms[3]
		default:
			return nil, fmt.Errorf("CASERECV構文が不正です(書式: CASERECV l1 (l2) cs): %s", line)
		}
		var lhs []ast.Expr
		for _, d := range destAtoms {
			e, err := atomExpr(d, funcName, closureLevel, CatMulti)
			if err != nil {
				return nil, fmt.Errorf("CASERECVの代入先が不正です: %w", err)
			}
			lhs = append(lhs, e)
		}
		ch, err := atomExpr(chAtom, funcName, closureLevel, CatSingle)
		if err != nil {
			return nil, fmt.Errorf("CASERECVのチャネルが不正です: %w", err)
		}
		return &ast.AssignStmt{
			Lhs: lhs, Tok: token.ASSIGN,
			Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.ARROW, X: ch}},
		}, nil

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
		typExpr, err := atomExpr(typeAtom, "", 0, CatType)
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
	elemExpr, err := atomExpr(atoms[1], "", 0, CatType)
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
	elemExpr, err := atomExpr(atoms[1], "", 0, CatType)
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
	keyExpr, err := atomExpr(atoms[1], "", 0, CatType)
	if err != nil {
		return nil, fmt.Errorf("MPTYPEのキー型が不正です: %w", err)
	}
	valExpr, err := atomExpr(atoms[2], "", 0, CatType)
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
