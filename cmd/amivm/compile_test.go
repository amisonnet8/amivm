package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGenerateOutput_SamePackageSiblingFunction は、amivmが生成するファイルと同じ
// ディレクトリ・同じpackageに置かれた手書きのGo関数をCALLで呼べることを検証する。
// importer.Default()を使っていた旧実装では、生成した1ファイルだけを単独でチェックして
// いたため、同じpackage内の他ファイルで定義された識別子が常にundefinedになっていた。
//
// go.mod無しのディレクトリでは`go build`自体が単一ファイルモードになり隣接ファイルを
// 見ないため(標準のGoの挙動)、この検証には出力先ディレクトリがGoモジュールである
// ことが前提になる。
func TestGenerateOutput_SamePackageSiblingFunction(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module xxlangtestmod2\n\ngo 1.26\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	helperSrc := "package main\n\n" +
		"func xxlangHelper(a int, b int) int {\n" +
		"\treturn a*10 + b\n" +
		"}\n"
	if err := os.WriteFile(filepath.Join(dir, "helper.go"), []byte(helperSrc), 0644); err != nil {
		t.Fatalf("write helper.go: %v", err)
	}

	ir := "FUNC\t!main\t:\n" +
		"\tVAR\t%r\t^int\n" +
		"\tCALL\t%r\t:\t?xxlangHelper\t1\t2\n" +
		"\tCALL\t:\t?fmt.Println\t%r\n" +
		"\tRET\n" +
		"ENDFUNC\n"

	file, err := buildProgram(ir)
	if err != nil {
		t.Fatalf("buildProgram: %v", err)
	}

	outputPath := filepath.Join(dir, "main.go")
	if err := generateOutput(file, outputPath, false); err != nil {
		t.Fatalf("generateOutput: %v (同一package内の手書き関数を解決できるべき)", err)
	}
}

// TestTypeCheck_CrossPackageSameModule は、amivmが生成するファイルと同じモジュール内に
// ある別packageへの参照(import済み)を型チェック段階で解決できることを検証する。
// importer.Default()はGoモジュールを理解しないため、旧実装ではこの参照は常に
// 「could not import」エラーになっていた。
//
// typeCheckを直接呼び、import文を含む完成済みソースを渡す形でテストする。
// buildProgram→generateOutputの通常経路だとimport文の自動挿入(goimports)を経由するが、
// goimportsの「識別子からimportパスを推測する」機構は、まだどこからも参照されていない
// 別モジュール内パッケージに対しては信頼できない(挿入に失敗する、あるいは誤ったパスを
// 挿入することがある)。これは今回直した「import済みの参照をgo/typesで解決する」層とは
// 別の問題であり、本テストの対象外(既知の別課題として切り分ける)。
func TestTypeCheck_CrossPackageSameModule(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module xxlangtestmod\n\ngo 1.26\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "xxrt"), 0755); err != nil {
		t.Fatalf("mkdir xxrt: %v", err)
	}
	xxrtSrc := "package xxrt\n\n" +
		"func Helper(a int, b int) int {\n" +
		"\treturn a*10 + b\n" +
		"}\n"
	if err := os.WriteFile(filepath.Join(dir, "xxrt", "xxrt.go"), []byte(xxrtSrc), 0644); err != nil {
		t.Fatalf("write xxrt.go: %v", err)
	}

	mainSrc := "package main\n\n" +
		"import (\n" +
		"\t\"fmt\"\n\n" +
		"\t\"xxlangtestmod/xxrt\"\n" +
		")\n\n" +
		"func main() {\n" +
		"\tr := xxrt.Helper(1, 2)\n" +
		"\tfmt.Println(r)\n" +
		"}\n"

	outputPath := filepath.Join(dir, "main.go")
	unusedVars, otherErrs, err := typeCheck(outputPath, []byte(mainSrc))
	if err != nil {
		t.Fatalf("typeCheck: %v", err)
	}
	if len(otherErrs) != 0 {
		t.Fatalf("typeCheck otherErrs = %v, 空であるべき(同一モジュール内の別packageをimport済みなら解決できるべき)", otherErrs)
	}
	if len(unusedVars) != 0 {
		t.Fatalf("typeCheck unusedVars = %v, 空であるべき", unusedVars)
	}
}
