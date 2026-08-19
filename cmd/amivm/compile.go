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
	"regexp"
	"strings"

	"golang.org/x/tools/imports"
)

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
