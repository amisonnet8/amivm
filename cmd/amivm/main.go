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

const usage = "Usage: amivm <ir-file-path> [-o|--output <output-file-path>] [-v|--verbose] [-i|--import <name>=<import-path>]... [-h|--help]"

const helpText = usage + `

amivm compiles an AMIVM-IR file into Go source code.

Arguments:
  <ir-file-path>                Path to the AMIVM-IR file to compile

Options:
  -o, --output <path>           Output file path (default: <ir-file-path> with its extension replaced by .go)
  -v, --verbose                 Print the input IR, the unused-variable self-repair log, and the final generated code
  -i, --import <name>=<path>    Add an explicit import "<path>" bound to <name> in the generated code; may be repeated
  -h, --help                    Show this help message and exit
`

// hasHelpFlag は引数リストに -h/--help が含まれるかを調べる。
// -h/--help は他のオプションの妥当性(name=path形式など)より先にチェックし、
// <IRファイルパス>を伴わない`amivm -h`のような呼び出しでもヘルプを表示できるようにする。
func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

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
		return "", "", fmt.Errorf("invalid -i/--import format (expected name=path): %s", raw)
	}
	name, path = raw[:idx], raw[idx+1:]
	if !reImportName.MatchString(name) {
		return "", "", fmt.Errorf("invalid -i/--import name (must be a valid Go identifier): %s", name)
	}
	if path == "" {
		return "", "", fmt.Errorf("empty import path for -i/--import: %s", raw)
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
				return "", "", false, nil, fmt.Errorf("-o/--output requires an output file path")
			}
			i++
			outPath = args[i]
		case "-v", "--verbose":
			verbose = true
		case "-i", "--import":
			if i+1 >= len(args) {
				return "", "", false, nil, fmt.Errorf("-i/--import requires a name=path argument")
			}
			i++
			name, path, parseErr := parseImportArg(args[i])
			if parseErr != nil {
				return "", "", false, nil, parseErr
			}
			if _, dup := importMap[name]; dup {
				return "", "", false, nil, fmt.Errorf("duplicate name specified for -i/--import: %s", name)
			}
			importMap[name] = path
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", false, nil, fmt.Errorf("unknown option: %s", args[i])
			}
			if irPath != "" {
				return "", "", false, nil, fmt.Errorf("only one IR file path may be specified: %s", args[i])
			}
			irPath = args[i]
		}
	}
	if irPath == "" {
		return "", "", false, nil, fmt.Errorf("an IR file path must be specified")
	}
	return irPath, outPath, verbose, importMap, nil
}

// injectExplicitImports は、-i/--importで指定されたインポートをfile.Declsの先頭に明示的な
// import宣言として追加する。goimports(imports.Process)は識別子だけからimportパスを
// 正しく推測できないことがあるため(標準ライブラリや既に参照済みのパッケージ以外では
// 信頼できない)、呼び出し側が既知のマッピングを明示的に渡せるようにする。ここで追加した
// エイリアスのうち実際にコード内で使われていないものは、後段のimports.Processが通常の
// 未使用import除去と同じ仕組みで自動的に取り除く。
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
	if hasHelpFlag(os.Args[1:]) {
		fmt.Print(helpText)
		return
	}

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
		fmt.Printf("failed to read IR file (%s): %v\n", irPath, err)
		os.Exit(1)
	}
	irSource := string(irBytes)

	if verbose {
		fmt.Println("=== IR ===")
		fmt.Println(irSource)
	}

	file, err := buildProgram(irSource)
	if err != nil {
		fmt.Println("IR parse error:", err)
		os.Exit(1)
	}

	if len(importMap) > 0 {
		if verbose {
			fmt.Println("=== explicit imports specified via -i/--import ===")
			for name, path := range importMap {
				fmt.Printf("%s = %q\n", name, path)
			}
		}
		injectExplicitImports(file, importMap)
	}

	if err := generateOutput(file, outPath, verbose); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	if verbose {
		fmt.Printf("generated successfully: %s\n", outPath)
	}
}
