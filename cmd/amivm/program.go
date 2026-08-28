package main

import (
	"fmt"
	"go/ast"
)

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
			defName, typeParams, params, results, err := parseFuncSignature(line)
			if err != nil {
				return nil, err
			}
			body, next, _, err := parseBody(lines, i+1, defName, 0, []string{"ENDFUNC"})
			if err != nil {
				return nil, fmt.Errorf("関数 %s のパースに失敗: %w", defName, err)
			}
			funcDecl := &ast.FuncDecl{
				Name: ast.NewIdent(amivmFuncGoName(defName)),
				Type: &ast.FuncType{
					TypeParams: typeParams,
					Params:     &ast.FieldList{List: params},
					Results:    fieldListOrNil(results),
				},
				Body: &ast.BlockStmt{List: body},
			}
			decls = append(decls, funcDecl)
			i = next

		case "FUNCM":
			defName, recv, params, results, err := parseFuncmSignature(line)
			if err != nil {
				return nil, err
			}
			body, next, _, err := parseBody(lines, i+1, defName, 0, []string{"ENDFUNCM"})
			if err != nil {
				return nil, fmt.Errorf("メソッド %s のパースに失敗: %w", defName, err)
			}
			funcDecl := &ast.FuncDecl{
				Recv: &ast.FieldList{List: []*ast.Field{recv}},
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
			rest := atoms[1:]
			if len(rest) == 0 {
				return nil, fmt.Errorf("STTYPE構文が不正です(型名がありません): %s", line)
			}
			deftypeAtom := rest[0]
			if err := checkKind(deftypeAtom, CatTypename); err != nil {
				return nil, fmt.Errorf("STTYPEの型名が不正です: %w", err)
			}
			typeParams, err := parseTypeParamPairs(rest[1:])
			if err != nil {
				return nil, fmt.Errorf("STTYPEの型パラメータが不正です: %w", err)
			}
			structType, next, err := parseStructBlock(lines, i+1)
			if err != nil {
				return nil, err
			}
			decls = append(decls, typeDecl(deftypeAtom.A, typeParams, structType))
			i = next

		case "INTYPE":
			atoms := tokenizeAndClassify(line)
			rest := atoms[1:]
			if len(rest) == 0 {
				return nil, fmt.Errorf("INTYPE構文が不正です(型名がありません): %s", line)
			}
			nameAtom := rest[0]
			if err := checkKind(nameAtom, CatTypename); err != nil {
				return nil, fmt.Errorf("INTYPEの型名が不正です: %w", err)
			}
			typeParams, err := parseTypeParamPairs(rest[1:])
			if err != nil {
				return nil, fmt.Errorf("INTYPEの型パラメータが不正です: %w", err)
			}
			ifaceType, next, err := parseInterfaceBlock(lines, i+1)
			if err != nil {
				return nil, err
			}
			decls = append(decls, typeDecl(nameAtom.A, typeParams, ifaceType))
			i = next

		case "GETYPE":
			atoms := tokenizeAndClassify(line)
			decl, err := parseGetype(atoms[1:])
			if err != nil {
				return nil, err
			}
			decls = append(decls, decl)
			i++

		default:
			return nil, fmt.Errorf("トップレベルにはGVAR/FUNC/FUNCM/CHTYPE/SLTYPE/STTYPE/INTYPE/MPTYPE/FNTYPE/GETYPEのみ置けます。不正な行: %s", line)
		}
	}

	return &ast.File{Name: ast.NewIdent("main"), Decls: decls}, nil
}
