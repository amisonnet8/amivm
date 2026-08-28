package main

import (
	"fmt"
	"go/ast"
)

// =====================================================================
// プログラム全体の組み立て
// トップレベルには GVAR / FUNC / ARTYPE / CHTYPE / SLTYPE / STTYPE / MPTYPE / FNTYPE のみ置ける
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
				return nil, fmt.Errorf("failed to parse function %s: %w", defName, err)
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
				return nil, fmt.Errorf("failed to parse method %s: %w", defName, err)
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

		case "ARTYPE":
			atoms := tokenizeAndClassify(line)
			decl, err := parseArType(atoms[1:])
			if err != nil {
				return nil, err
			}
			decls = append(decls, decl)
			i++

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
				return nil, fmt.Errorf("invalid STTYPE syntax (missing type name): %s", line)
			}
			deftypeAtom := rest[0]
			if err := checkKind(deftypeAtom, CatTypename); err != nil {
				return nil, fmt.Errorf("invalid STTYPE type name: %w", err)
			}
			typeParams, err := parseTypeParamPairs(rest[1:])
			if err != nil {
				return nil, fmt.Errorf("invalid STTYPE type parameter: %w", err)
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
				return nil, fmt.Errorf("invalid INTYPE syntax (missing type name): %s", line)
			}
			nameAtom := rest[0]
			if err := checkKind(nameAtom, CatTypename); err != nil {
				return nil, fmt.Errorf("invalid INTYPE type name: %w", err)
			}
			typeParams, err := parseTypeParamPairs(rest[1:])
			if err != nil {
				return nil, fmt.Errorf("invalid INTYPE type parameter: %w", err)
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
			return nil, fmt.Errorf("only GVAR/FUNC/FUNCM/ARTYPE/CHTYPE/SLTYPE/STTYPE/INTYPE/MPTYPE/FNTYPE/GETYPE are allowed at the top level. invalid line: %s", line)
		}
	}

	return &ast.File{Name: ast.NewIdent("main"), Decls: decls}, nil
}
