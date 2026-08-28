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

func typeDecl(name string, typeParams *ast.FieldList, typ ast.Expr) ast.Decl {
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{Name: ast.NewIdent(name), TypeParams: typeParams, Type: typ},
		},
	}
}

// typeAliasDecl は「type name = typ」(エイリアス宣言)を組み立てる。GETYPEで使う。
// ast.TypeSpec.Assignは、go/printerが「非ゼロなら=を付ける」とだけ見る位置情報
// フィールドのため、実際のソース上の位置を持たないASTでも token.Pos(1) のような
// 非ゼロ値を入れておけばエイリアス構文として出力される。
func typeAliasDecl(name string, typ ast.Expr) ast.Decl {
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{Name: ast.NewIdent(name), Assign: token.Pos(1), Type: typ},
		},
	}
}

// parseTypeParamPairs は「typename1 constraint1 typename2 constraint2 ...」
// (typename+constraintのペアの並び)を検証し、型パラメータ宣言の*ast.FieldListに
// 変換する。FUNC/STTYPE/INTYPEの型パラメータ宣言セグメントで共用する。atomsが
// 空なら(型パラメータ無し)nilを返す。
func parseTypeParamPairs(atoms []Atom) (*ast.FieldList, error) {
	if len(atoms) == 0 {
		return nil, nil
	}
	if len(atoms)%2 != 0 {
		return nil, fmt.Errorf("type parameters must be specified as name/constraint pairs: %s", joinRaw(atoms))
	}
	var fields []*ast.Field
	for i := 0; i < len(atoms); i += 2 {
		nameAtom, constraintAtom := atoms[i], atoms[i+1]
		if err := checkKind(nameAtom, CatTypename); err != nil {
			return nil, fmt.Errorf("invalid type parameter name: %w", err)
		}
		constraintExpr, err := atomExpr(constraintAtom, "", 0, CatConstraint)
		if err != nil {
			return nil, fmt.Errorf("invalid type parameter constraint: %w", err)
		}
		fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(nameAtom.A)}, Type: constraintExpr})
	}
	return &ast.FieldList{List: fields}, nil
}

// parseFuncSignature は「FUNC defname (typename1 constraint1 ... :) type1 type2 ... : type3 type4 ...」
// を解釈する。defnameの後をコロンの個数で分割する: コロン1個なら型パラメータ無し
// (パラメータ型:戻り値型)、2個なら先頭セグメントが型パラメータ宣言。
func parseFuncSignature(line string) (defName string, typeParams *ast.FieldList, params []*ast.Field, results []*ast.Field, err error) {
	atoms := tokenizeAndClassify(line)
	if len(atoms) == 0 || atoms[0].Raw != "FUNC" {
		return "", nil, nil, nil, fmt.Errorf("invalid FUNC syntax: %s", line)
	}
	rest := atoms[1:]
	if len(rest) == 0 {
		return "", nil, nil, nil, fmt.Errorf("invalid FUNC syntax (missing function name): %s", line)
	}
	defAtom := rest[0]
	if err := checkKind(defAtom, CatDefname); err != nil {
		return "", nil, nil, nil, fmt.Errorf("invalid FUNC function name: %w", err)
	}
	defName, err = amivmFuncNameOf(defAtom)
	if err != nil {
		return "", nil, nil, nil, err
	}
	segments := splitByColons(rest[1:])
	var typeParamAtoms, paramAtoms, resultAtoms []Atom
	switch len(segments) {
	case 2:
		paramAtoms, resultAtoms = segments[0], segments[1]
	case 3:
		typeParamAtoms, paramAtoms, resultAtoms = segments[0], segments[1], segments[2]
	default:
		return "", nil, nil, nil, fmt.Errorf("invalid FUNC syntax: %s", line)
	}
	typeParams, err = parseTypeParamPairs(typeParamAtoms)
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("invalid FUNC type parameter: %w", err)
	}
	results, err = typeFieldsUnnamed(resultAtoms)
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("invalid FUNC return type: %w", err)
	}
	params, err = typeFieldsNamed(paramAtoms, func(i int) string { return amivmParamGoName(i + 1) })
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("invalid FUNC parameter type: %w", err)
	}
	return defName, typeParams, params, results, nil
}

// buildReceiverTypeExpr は、FUNCMのレシーバー型Atom(CatReceiver: ^xxx / ^*xxx)と、
// (あれば)型パラメータ再掲リスト(STTYPE側で宣言済みの型パラメータ名のベアな並び。
// 制約は書かない)から、レシーバーのast.Exprを組み立てる。例: レシーバーが^*Boxで
// 再掲が[T]なら *Box[T]。再掲が無ければそのままのポインタ有無で返す。
func buildReceiverTypeExpr(recvAtom Atom, reuseAtoms []Atom) (ast.Expr, error) {
	if err := checkKind(recvAtom, CatReceiver); err != nil {
		return nil, fmt.Errorf("invalid FUNCM receiver type: %w", err)
	}
	var typeArgs []ast.Expr
	for _, a := range reuseAtoms {
		if err := checkKind(a, CatTypename); err != nil {
			return nil, fmt.Errorf("invalid FUNCM type parameter restatement: %w", err)
		}
		typeArgs = append(typeArgs, ast.NewIdent(a.A))
	}
	switch recvAtom.Kind {
	case KType:
		return instantiateTypeExpr(ast.NewIdent(recvAtom.A), typeArgs), nil
	case KTypePtr:
		return &ast.StarExpr{X: instantiateTypeExpr(ast.NewIdent(recvAtom.A), typeArgs)}, nil
	default:
		return nil, fmt.Errorf("invalid FUNCM receiver type: %s", recvAtom.Raw)
	}
}

// parseFuncmSignature は「FUNCM defname receiver (typename1 typename2 ... :) type1 type2 ... : type3 type4 ...」
// を解釈する。receiverの後をコロンの個数で分割する: コロン1個なら型パラメータ
// 再掲無し、2個なら先頭セグメントが型パラメータ再掲(ベア名の並び。制約は書かない。
// STTYPE側で宣言済みの型パラメータ名をそのまま再掲するだけのため)。レシーバーの
// Go側の実名は常に amivm_method_self(関数名による修飾は無い。FUNCM本体内では
// $0として参照する)。
func parseFuncmSignature(line string) (defName string, recv *ast.Field, params, results []*ast.Field, err error) {
	atoms := tokenizeAndClassify(line)
	if len(atoms) == 0 || atoms[0].Raw != "FUNCM" {
		return "", nil, nil, nil, fmt.Errorf("invalid FUNCM syntax: %s", line)
	}
	rest := atoms[1:]
	if len(rest) < 2 {
		return "", nil, nil, nil, fmt.Errorf("invalid FUNCM syntax (missing function name/receiver): %s", line)
	}
	defAtom := rest[0]
	if err := checkKind(defAtom, CatDefname); err != nil {
		return "", nil, nil, nil, fmt.Errorf("invalid FUNCM function name: %w", err)
	}
	defName, err = amivmFuncNameOf(defAtom)
	if err != nil {
		return "", nil, nil, nil, err
	}
	recvAtom := rest[1]
	segments := splitByColons(rest[2:])
	var reuseAtoms, paramAtoms, resultAtoms []Atom
	switch len(segments) {
	case 2:
		paramAtoms, resultAtoms = segments[0], segments[1]
	case 3:
		reuseAtoms, paramAtoms, resultAtoms = segments[0], segments[1], segments[2]
	default:
		return "", nil, nil, nil, fmt.Errorf("invalid FUNCM syntax: %s", line)
	}
	recvTypeExpr, err := buildReceiverTypeExpr(recvAtom, reuseAtoms)
	if err != nil {
		return "", nil, nil, nil, err
	}
	recv = &ast.Field{Names: []*ast.Ident{ast.NewIdent("amivm_method_self")}, Type: recvTypeExpr}
	results, err = typeFieldsUnnamed(resultAtoms)
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("invalid FUNCM return type: %w", err)
	}
	params, err = typeFieldsNamed(paramAtoms, func(i int) string { return amivmParamGoName(i + 1) })
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("invalid FUNCM parameter type: %w", err)
	}
	return defName, recv, params, results, nil
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
		return nil, nil, 0, fmt.Errorf("invalid CLOS syntax: %s", line)
	}
	rest := atoms[1:]
	if len(rest) == 0 {
		return nil, nil, 0, fmt.Errorf("invalid CLOS syntax (missing assignment target variable): %s", line)
	}
	targetAtom := rest[0]
	targetExpr, err = atomExpr(targetAtom, funcName, currentLevel, CatSingle)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("invalid CLOS assignment target: %w", err)
	}
	paramAtoms, resultAtoms, err := splitColon(rest[1:])
	if err != nil {
		return nil, nil, 0, fmt.Errorf("invalid CLOS syntax: %w", err)
	}
	results, err := typeFieldsUnnamed(resultAtoms)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("invalid CLOS return type: %w", err)
	}
	newLevel = currentLevel + 1
	params, err := typeFieldsNamed(paramAtoms, func(i int) string { return closureParamGoName(newLevel, i+1) })
	if err != nil {
		return nil, nil, 0, fmt.Errorf("invalid CLOS parameter type: %w", err)
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
			return nil, 0, "", fmt.Errorf("%s cannot be used outside an IF, or after an ELSE: %s", kw, line)
		case "ENDIF":
			return nil, 0, "", fmt.Errorf("no matching IF found: %s", line)
		case "ENDLOOP":
			return nil, 0, "", fmt.Errorf("no matching LOOP found: %s", line)
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
	return nil, 0, "", fmt.Errorf("no matching %s found", strings.Join(blockEnds, " or "))
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
		return nil, 0, fmt.Errorf("invalid IF/ELIF syntax (format: IF boolean1 / ELIF boolean1): %s", lines[headerIdx])
	}
	kw := atoms[0].Raw
	cond, err := atomExpr(atoms[1], funcName, closureLevel, CatBool)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid %s condition: %w", kw, err)
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
		return nil, 0, fmt.Errorf("invalid LOOP syntax (format: LOOP): %s", lines[loopIdx])
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
			return nil, 0, fmt.Errorf("no matching ENDSEL found")
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
		return nil, fmt.Errorf("blank lines are not allowed inside SEL")
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
			return nil, fmt.Errorf("invalid CASESEND channel: %w", err)
		}
		v, err := atomExpr(atoms[2], funcName, closureLevel, CatValue)
		if err != nil {
			return nil, fmt.Errorf("invalid CASESEND send value: %w", err)
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
			return nil, fmt.Errorf("invalid CASERECV syntax (format: CASERECV l1 (l2) cs): %s", line)
		}
		var lhs []ast.Expr
		for _, d := range destAtoms {
			e, err := atomExpr(d, funcName, closureLevel, CatMulti)
			if err != nil {
				return nil, fmt.Errorf("invalid CASERECV assignment target: %w", err)
			}
			lhs = append(lhs, e)
		}
		ch, err := atomExpr(chAtom, funcName, closureLevel, CatSingle)
		if err != nil {
			return nil, fmt.Errorf("invalid CASERECV channel: %w", err)
		}
		return &ast.AssignStmt{
			Lhs: lhs, Tok: token.ASSIGN,
			Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.ARROW, X: ch}},
		}, nil

	default:
		return nil, fmt.Errorf("only CASESEND/CASERECV/DEFAULT can be used inside SEL: %s", line)
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
			return nil, 0, fmt.Errorf("only FIELD can be used inside STTYPE: %s", line)
		}
		atoms := tokenizeAndClassify(line)
		if err := expectArgs("FIELD", atoms[1:], 2); err != nil {
			return nil, 0, err
		}
		fieldAtom, typeAtom := atoms[1], atoms[2]
		if fieldAtom.Kind != KField {
			return nil, 0, fmt.Errorf("invalid FIELD field name: %s", fieldAtom.Raw)
		}
		typExpr, err := atomExpr(typeAtom, "", 0, CatType)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid FIELD type: %w", err)
		}
		fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(fieldAtom.A)}, Type: typExpr})
		i++
	}
	return nil, 0, fmt.Errorf("no matching ENDSTTYPE found")
}

// parseInterfaceBlock は INTYPE の本体(METHOD行の並び)をENDINTYPEまで読み取る。
// parseStructBlockと同型だが、各行は「METHOD method type1 ... : type3 ...」という
// メソッドシグネチャ(レシーバー無し・本体無し)で、コロンは常に必須(type1 type2 ...
// がいつ終わるか構文上判別できないため、他のコロン区切り命令と同じ厳格ルール)。
// この命令名METHODは、旧METHOD(現METHVAL)の改名で空いたキーワードを再利用した
// もので、parseBody経由のグローバルなディスパッチ(parseSingleLine)は通らない
// (INTYPE本体はこの専用ループでしか読まれない)ため衝突しない。
func parseInterfaceBlock(lines []string, start int) (*ast.InterfaceType, int, error) {
	var methods []*ast.Field
	i := start
	for i < len(lines) {
		line := lines[i]
		kw := keyword(line)
		if kw == "ENDINTYPE" {
			return &ast.InterfaceType{Methods: &ast.FieldList{List: methods}}, i + 1, nil
		}
		if kw == "" {
			i++
			continue
		}
		if kw != "METHOD" {
			return nil, 0, fmt.Errorf("only METHOD can be used inside INTYPE: %s", line)
		}
		atoms := tokenizeAndClassify(line)
		rest := atoms[1:]
		if len(rest) == 0 {
			return nil, 0, fmt.Errorf("invalid METHOD syntax (missing method name): %s", line)
		}
		methodAtom := rest[0]
		if methodAtom.Kind != KMethod {
			return nil, 0, fmt.Errorf("invalid METHOD method name: %s", methodAtom.Raw)
		}
		paramAtoms, resultAtoms, err := splitColon(rest[1:])
		if err != nil {
			return nil, 0, fmt.Errorf("invalid METHOD syntax: %w", err)
		}
		params, err := typeFieldsUnnamed(paramAtoms)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid METHOD parameter type: %w", err)
		}
		results, err := typeFieldsUnnamed(resultAtoms)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid METHOD return type: %w", err)
		}
		methods = append(methods, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(methodAtom.A)},
			Type:  &ast.FuncType{Params: &ast.FieldList{List: params}, Results: fieldListOrNil(results)},
		})
		i++
	}
	return nil, 0, fmt.Errorf("no matching ENDINTYPE found")
}

// parseChType/parseSlType/parseMpType/parseFnType はトップレベルのTYPE系宣言
// (CHTYPE/SLTYPE/MPTYPE/FNTYPE)を1行から*ast.Declに変換する。

func parseChType(atoms []Atom) (ast.Decl, error) {
	if err := expectArgs("CHTYPE", atoms, 2); err != nil {
		return nil, err
	}
	if err := checkKind(atoms[0], CatTypename); err != nil {
		return nil, fmt.Errorf("invalid CHTYPE type name: %w", err)
	}
	elemExpr, err := atomExpr(atoms[1], "", 0, CatType)
	if err != nil {
		return nil, fmt.Errorf("invalid CHTYPE element type: %w", err)
	}
	return typeDecl(atoms[0].A, nil, chanTypeExpr(elemExpr)), nil
}

func parseSlType(atoms []Atom) (ast.Decl, error) {
	if err := expectArgs("SLTYPE", atoms, 2); err != nil {
		return nil, err
	}
	if err := checkKind(atoms[0], CatTypename); err != nil {
		return nil, fmt.Errorf("invalid SLTYPE type name: %w", err)
	}
	elemExpr, err := atomExpr(atoms[1], "", 0, CatType)
	if err != nil {
		return nil, fmt.Errorf("invalid SLTYPE element type: %w", err)
	}
	return typeDecl(atoms[0].A, nil, sliceTypeExpr(elemExpr)), nil
}

func parseMpType(atoms []Atom) (ast.Decl, error) {
	if err := expectArgs("MPTYPE", atoms, 3); err != nil {
		return nil, err
	}
	if err := checkKind(atoms[0], CatTypename); err != nil {
		return nil, fmt.Errorf("invalid MPTYPE type name: %w", err)
	}
	keyExpr, err := atomExpr(atoms[1], "", 0, CatType)
	if err != nil {
		return nil, fmt.Errorf("invalid MPTYPE key type: %w", err)
	}
	valExpr, err := atomExpr(atoms[2], "", 0, CatType)
	if err != nil {
		return nil, fmt.Errorf("invalid MPTYPE value type: %w", err)
	}
	return typeDecl(atoms[0].A, nil, &ast.MapType{Key: keyExpr, Value: valExpr}), nil
}

func parseFnType(atoms []Atom) (ast.Decl, error) {
	if len(atoms) == 0 {
		return nil, fmt.Errorf("invalid FNTYPE syntax (missing type name)")
	}
	deftypeAtom := atoms[0]
	if err := checkKind(deftypeAtom, CatTypename); err != nil {
		return nil, fmt.Errorf("invalid FNTYPE type name: %w", err)
	}
	paramAtoms, resultAtoms, err := splitColon(atoms[1:])
	if err != nil {
		return nil, fmt.Errorf("invalid FNTYPE syntax: %w", err)
	}
	params, err := typeFieldsUnnamed(paramAtoms)
	if err != nil {
		return nil, fmt.Errorf("invalid FNTYPE parameter type: %w", err)
	}
	results, err := typeFieldsUnnamed(resultAtoms)
	if err != nil {
		return nil, fmt.Errorf("invalid FNTYPE return type: %w", err)
	}
	ft := &ast.FuncType{Params: &ast.FieldList{List: params}, Results: fieldListOrNil(results)}
	return typeDecl(deftypeAtom.A, nil, ft), nil
}

// parseGetype は「GETYPE typename1 typename2 type1 type2 ...」を解釈する
// (type typename1 = typename2[type1, type2, ...])。typename1が新しく宣言する
// 別名、typename2が実体化する対象のジェネリクス型名、type1以降が当てはめる型引数
// (1個以上必須)。
func parseGetype(atoms []Atom) (ast.Decl, error) {
	if len(atoms) < 3 {
		return nil, fmt.Errorf("invalid GETYPE syntax (format: GETYPE typename1 typename2 type1 ...): %s", joinRaw(atoms))
	}
	aliasAtom, targetAtom, typeArgAtoms := atoms[0], atoms[1], atoms[2:]
	if err := checkKind(aliasAtom, CatTypename); err != nil {
		return nil, fmt.Errorf("invalid GETYPE alias: %w", err)
	}
	if err := checkKind(targetAtom, CatTypename); err != nil {
		return nil, fmt.Errorf("invalid GETYPE target type name: %w", err)
	}
	typeArgs, err := typeExprList(typeArgAtoms)
	if err != nil {
		return nil, fmt.Errorf("invalid GETYPE type argument: %w", err)
	}
	target := instantiateTypeExpr(ast.NewIdent(targetAtom.A), typeArgs)
	return typeAliasDecl(aliasAtom.A, target), nil
}
