package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// =====================================================================
// エントリポイント
// =====================================================================

const usage = "使い方: amivm <IRファイルパス> [-o|--output <出力ファイルパス>] [-v|--verbose] [-i|--import <名前>=<importパス>]..."

// deriveOutputPath は -o 未指定時の出力パスを決める。
// IRファイルパスの拡張子を.goに置き換える。拡張子が無ければ.goを付け足す。
func deriveOutputPath(irPath string) string {
	ext := filepath.Ext(irPath)
	if ext == "" {
		return irPath + ".go"
	}
	return strings.TrimSuffix(irPath, ext) + ".go"
}

// reImportName は -i/--import <名前>=<importパス> の<名前>側が満たすべき形式(Goの識別子)。
var reImportName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseImportArg は "name=path" 形式の -i/--import 引数1つを分解する。
func parseImportArg(raw string) (name, path string, err error) {
	idx := strings.Index(raw, "=")
	if idx < 0 {
		return "", "", fmt.Errorf("-i/--importの形式が不正です(name=path形式で指定してください): %s", raw)
	}
	name, path = raw[:idx], raw[idx+1:]
	if !reImportName.MatchString(name) {
		return "", "", fmt.Errorf("-i/--importの名前が不正です(Goの識別子である必要があります): %s", name)
	}
	if path == "" {
		return "", "", fmt.Errorf("-i/--importのimportパスが空です: %s", raw)
	}
	return name, path, nil
}

// parseArgs はコマンドライン引数を解釈する。<IRファイルパス>・-o/--output・-v/--verbose・
// -i/--importの順序は問わない。短縮形と長形式は完全に同じ意味を持つ(どちらを使ってもよい)。
// -i/--importは繰り返し指定できる(利用側言語実装が用意する独自のランタイムライブラリ等、
// goimportsが識別子だけからは正しく推測できないimportパスを明示するためのもの)。
func parseArgs(args []string) (irPath, outPath string, verbose bool, importMap map[string]string, err error) {
	importMap = map[string]string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--output":
			if i+1 >= len(args) {
				return "", "", false, nil, fmt.Errorf("-o/--outputオプションには出力ファイルパスの指定が必要です")
			}
			i++
			outPath = args[i]
		case "-v", "--verbose":
			verbose = true
		case "-i", "--import":
			if i+1 >= len(args) {
				return "", "", false, nil, fmt.Errorf("-i/--importオプションにはname=path形式の指定が必要です")
			}
			i++
			name, path, parseErr := parseImportArg(args[i])
			if parseErr != nil {
				return "", "", false, nil, parseErr
			}
			if _, dup := importMap[name]; dup {
				return "", "", false, nil, fmt.Errorf("-i/--importで同じ名前が複数回指定されています: %s", name)
			}
			importMap[name] = path
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", false, nil, fmt.Errorf("不明なオプションです: %s", args[i])
			}
			if irPath != "" {
				return "", "", false, nil, fmt.Errorf("IRファイルパスは1つだけ指定してください: %s", args[i])
			}
			irPath = args[i]
		}
	}
	if irPath == "" {
		return "", "", false, nil, fmt.Errorf("IRファイルパスを指定してください")
	}
	return irPath, outPath, verbose, importMap, nil
}

// injectExplicitImports は、-i/--importで指定されたインポートをfile.Declsの先頭に明示的な
// import宣言として追加する。goimports(imports.Process)は識別子だけからimportパスを
// 正しく推測できないことがあるため(標準ライブラリや既に参照済みのパッケージ以外では
// 信頼できない。docs/amivm_code_design.md「既知の別課題」参照)、呼び出し側が既知の
// マッピングを明示的に渡せるようにする。ここで追加したエイリアスのうち実際にコード内で
// 使われていないものは、後段のimports.Processが通常の未使用import除去と同じ仕組みで
// 自動的に取り除く。
func injectExplicitImports(file *ast.File, importMap map[string]string) {
	if len(importMap) == 0 {
		return
	}
	names := make([]string, 0, len(importMap))
	for name := range importMap {
		names = append(names, name)
	}
	sort.Strings(names)

	specs := make([]ast.Spec, 0, len(names))
	for _, name := range names {
		specs = append(specs, &ast.ImportSpec{
			Name: ast.NewIdent(name),
			Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(importMap[name])},
		})
	}
	importDecl := &ast.GenDecl{Tok: token.IMPORT, Specs: specs}
	file.Decls = append([]ast.Decl{importDecl}, file.Decls...)
}

func main() {
	irPath, outPath, verbose, importMap, err := parseArgs(os.Args[1:])
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

	if len(importMap) > 0 {
		if verbose {
			fmt.Println("=== -i/--importで指定された明示的import ===")
			for name, path := range importMap {
				fmt.Printf("%s = %q\n", name, path)
			}
		}
		injectExplicitImports(file, importMap)
	}

	if err := generateOutput(file, outPath, verbose); err != nil {
		fmt.Println("エラー:", err)
		os.Exit(1)
	}

	if verbose {
		fmt.Printf("生成成功: %s\n", outPath)
	}
}
