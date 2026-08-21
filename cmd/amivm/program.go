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
			defName, params, results, err := parseFuncSignature(line)
			if err != nil {
				return nil, err
			}
			body, next, err := parseBody(lines, i+1, defName, 0, "ENDFUNC")
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
