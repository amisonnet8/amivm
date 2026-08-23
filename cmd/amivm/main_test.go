package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseArgs_Import(t *testing.T) {
	// 短縮形(-o -v -i)・長形式(--output --verbose --import)は完全に同じ意味を持つべき。
	variants := [][]string{
		{
			"foo.ir", "-o", "foo.go", "-v",
			"-i", "xxrt=github.com/xxlang/xxrt",
			"-i", "util=github.com/xxlang/util",
		},
		{
			"foo.ir", "--output", "foo.go", "--verbose",
			"--import", "xxrt=github.com/xxlang/xxrt",
			"--import", "util=github.com/xxlang/util",
		},
		{
			"foo.ir", "-o", "foo.go", "--verbose",
			"-i", "xxrt=github.com/xxlang/xxrt",
			"--import", "util=github.com/xxlang/util",
		},
	}
	want := map[string]string{
		"xxrt": "github.com/xxlang/xxrt",
		"util": "github.com/xxlang/util",
	}
	for _, args := range variants {
		irPath, outPath, verbose, importMap, err := parseArgs(args)
		if err != nil {
			t.Fatalf("parseArgs(%v): %v", args, err)
		}
		if irPath != "foo.ir" || outPath != "foo.go" || !verbose {
			t.Fatalf("parseArgs(%v): irPath/outPath/verbose = %q/%q/%v, unexpected", args, irPath, outPath, verbose)
		}
		if len(importMap) != len(want) {
			t.Fatalf("parseArgs(%v): importMap = %v, want %v", args, importMap, want)
		}
		for name, path := range want {
			if importMap[name] != path {
				t.Errorf("parseArgs(%v): importMap[%q] = %q, want %q", args, name, importMap[name], path)
			}
		}
	}
}

func TestParseArgs_ImportErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"引数無し(短縮形)", []string{"foo.ir", "-i"}},
		{"引数無し(長形式)", []string{"foo.ir", "--import"}},
		{"=区切りが無い", []string{"foo.ir", "-i", "xxrt"}},
		{"名前がGo識別子でない", []string{"foo.ir", "-i", "1xxrt=github.com/xxlang/xxrt"}},
		{"パスが空", []string{"foo.ir", "-i", "xxrt="}},
		{"名前の重複(短縮形+長形式)", []string{"foo.ir", "-i", "xxrt=a", "--import", "xxrt=b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, _, _, err := parseArgs(c.args); err == nil {
				t.Fatalf("parseArgs(%v) succeeded, want error", c.args)
			}
		})
	}
}

// TestGenerateOutput_ImportFlagResolvesNeverBeforeReferencedPackage は、-i/--importで
// 明示したパッケージを実際にCALLで呼び出すIRが、buildProgram→injectExplicitImports→
// generateOutputという実際のCLIパイプラインを通して問題なく生成できることを検証する。
//
// このシナリオ(まだどこからも参照されていない別モジュール内パッケージ)は、
// goimportsの自動推測(imports.Process)だけに頼ると失敗する・不安定になることが
// あるため、-i/--importによる明示指定で確実に解決できることを確認する。
func TestGenerateOutput_ImportFlagResolvesNeverBeforeReferencedPackage(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module xxlangtestmod3\n\ngo 1.26\n"), 0644); err != nil {
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

	ir := "FUNC\t!main\t:\n" +
		"\tVAR\t%r\t^int\n" +
		"\tCALL\t%r\t:\t?xxrt.Helper\t1\t2\n" +
		"\tCALL\t:\t?fmt.Println\t%r\n" +
		"\tRET\n" +
		"ENDFUNC\n"

	file, err := buildProgram(ir)
	if err != nil {
		t.Fatalf("buildProgram: %v", err)
	}
	injectExplicitImports(file, map[string]string{"xxrt": "xxlangtestmod3/xxrt"})

	outputPath := filepath.Join(dir, "main.go")
	if err := generateOutput(file, outputPath, false); err != nil {
		t.Fatalf("generateOutput: %v (-i/--importで明示したパッケージを解決できるべき)", err)
	}
}
