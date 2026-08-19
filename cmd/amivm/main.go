package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// =====================================================================
// エントリポイント
// =====================================================================

const usage = "使い方: amivm <IRファイルパス> [-o <出力ファイルパス>] [-v]"

// deriveOutputPath は -o 未指定時の出力パスを決める。
// IRファイルパスの拡張子を.goに置き換える。拡張子が無ければ.goを付け足す。
func deriveOutputPath(irPath string) string {
	ext := filepath.Ext(irPath)
	if ext == "" {
		return irPath + ".go"
	}
	return strings.TrimSuffix(irPath, ext) + ".go"
}

// parseArgs はコマンドライン引数を解釈する。<IRファイルパス>・-o・-vの順序は問わない。
func parseArgs(args []string) (irPath, outPath string, verbose bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o":
			if i+1 >= len(args) {
				return "", "", false, fmt.Errorf("-oオプションには出力ファイルパスの指定が必要です")
			}
			i++
			outPath = args[i]
		case "-v":
			verbose = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", false, fmt.Errorf("不明なオプションです: %s", args[i])
			}
			if irPath != "" {
				return "", "", false, fmt.Errorf("IRファイルパスは1つだけ指定してください: %s", args[i])
			}
			irPath = args[i]
		}
	}
	if irPath == "" {
		return "", "", false, fmt.Errorf("IRファイルパスを指定してください")
	}
	return irPath, outPath, verbose, nil
}

func main() {
	irPath, outPath, verbose, err := parseArgs(os.Args[1:])
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

	if err := generateOutput(file, outPath, verbose); err != nil {
		fmt.Println("エラー:", err)
		os.Exit(1)
	}

	if verbose {
		fmt.Printf("生成成功: %s\n", outPath)
	}
}
