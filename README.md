# AMIVM

[![test](https://github.com/amisonnet8/amivm/actions/workflows/test.yml/badge.svg)](https://github.com/amisonnet8/amivm/actions/workflows/test.yml)

AMIVM is a compilation toolchain that translates a custom intermediate representation (**AMIVM-IR**) into Go source code. Go's concurrency primitives — goroutines and channels — are built directly into the IR, rather than bolted on afterward.

> [日本語版 README はこちら](README_ja.md)

## Status

This is a **prototype / work-in-progress implementation**. The whole compiler is a single Go package (no subdirectories) split across a handful of files by responsibility, and the front end (translating some target language into AMIVM-IR) is intentionally out of scope for this repository — see [Pipeline](#pipeline) below.

## Pipeline

```
target language
  ↓ (front end — not part of this repo)
AMIVM-IR
  ↓ AMIVM  (this repo)
Go source code
  ↓ go build
executable
```

AMIVM's own responsibility stops at emitting a Go source file. Turning that file into an executable is a separate, later step (a plain `go build`), not something the `amivm` command does itself.

## Requirements

- Go, matching the version in [`go.mod`](go.mod) (currently 1.26.5). Building pulls in `golang.org/x/tools/imports` for import resolution.

## Build

```sh
go build -o amivm ./cmd/amivm
```

or, using the provided `Makefile` (`make help` lists all targets, including `test`, which runs every example under `test_ir/` through the compiler):

```sh
make build
```

To install the latest released `amivm` without cloning this repo:

```sh
go install github.com/amisonnet8/amivm/cmd/amivm@latest
```

This lands in `$GOBIN` (or `$GOPATH/bin` if unset) — make sure that directory is on your `PATH`.

To use `amivm` from another project under development (e.g. a language front end targeting AMIVM-IR), install it onto your `PATH` from your local clone instead of copying the binary around:

```sh
make install   # go install ./cmd/amivm — lands in $GOBIN, or $GOPATH/bin if unset
```

Re-running `make install` after changes immediately updates the `amivm` the other project picks up, with no path coupling between the two repos. Where `amivm` ultimately gets deployed for end users (e.g. under `/usr/local/bin`) is a decision for that downstream project, not this one.

## Usage

```
amivm <ir-file> [-o|--output <output-file>] [-v|--verbose] [-i|--import <name>=<import-path>]...
```

| Option | Description |
|---|---|
| `-o <output-file>`, `--output <output-file>` | Where to write the generated Go file. If omitted, the output path is derived from `<ir-file>` by replacing its extension with `.go` (or appending `.go` if it has none). |
| `-v`, `--verbose` | Print progress (the source IR, self-healing steps, the final generated code, a success message). Without it, the command is silent on success — only errors are ever printed, with or without `-v`. |
| `-i <name>=<import-path>`, `--import <name>=<import-path>` | Repeatable. Adds an explicit, aliased `import <name> "<import-path>"` to the generated file up front, so a `?<name>.Func` call resolves to it directly instead of relying on goimports to guess the import path from the bare identifier — a guess that's only reliable for the standard library and packages already referenced elsewhere. Useful for calling into a custom Go library a downstream project (e.g. a language front end targeting AMIVM-IR) provides. Names that end up unused in the generated code are dropped automatically, so the same set of `-i`/`--import` flags can safely be reused across every IR file. |

## Example

```
FUNC	!main	:
	CALL	:	?fmt.Println	"Hello, AMIVM!"
	RET
ENDFUNC
```

```sh
$ amivm hello.ir -v
=== IR ===
...
=== 最終生成コード ===
package main

import "fmt"

func main() {
	fmt.Println("Hello, AMIVM!")
	return
}

生成成功: hello.go
$ go run hello.go
Hello, AMIVM!
```

More runnable examples covering every instruction, grouped by topic (variables, arithmetic, bitwise/shift/logical/comparison ops, strings, pointers, arrays with `GOTO`-based loops, functions and `DEFER`, goroutines/channels/`SEL`, slices, structs, maps, closures, Go method calls, structured `IF`/`LOOP` control flow, and type assertions), live in [`test_ir/`](test_ir/).

## The IR language, briefly

Every identifier carries a one-character prefix that fixes its kind up front:

| Prefix | Meaning |
|---|---|
| `$` | Function parameter |
| `&` | Closure parameter |
| `%` | Local (function-scoped) variable |
| `@` | Package-level variable |
| `^` | Type name |
| `>` | Struct field name |
| `!` | AMIVM-defined function name |
| `?` | Go function name |
| `#` | Label name |

Instructions are grouped roughly into:

- **Variables & assignment**: `VAR`, `GVAR`, `SET`
- **Arithmetic / bitwise / shift / logical / comparison**: `ADD` `SUB` `MUL` `DIV` `MOD` · `BAND` `BOR` `BXOR` `BCLEAR` `BNOT` · `SHL` `SHR` · `AND` `OR` `NOT` · `EQ` `NEQ` `LT` `LTE` `GT` `GTE`
- **Strings**: `CONCAT`, `SLICE`
- **Pointers**: `ADDR`, `PGET`, `PSET`
- **Arrays**: `ASET`, `AGET`
- **Labels & `goto`**: `LABEL`, `GOTO`
- **Conditionals**: `IF`/`ELIF`/`ELSE`/`ENDIF`
- **Loops**: `LOOP`/`BREAK`/`CONTINUE`/`ENDLOOP`
- **Type assertions**: `ASSERT`
- **Functions**: `FUNC`/`ENDFUNC`, `RET`, `CALL`, `DEFER`, `SPAWN`
- **Channels & `select`**: `CHTYPE`, `CHMAKE`, `CHSEND`, `CHRECV`, `SEL`/`ENDSEL`, `CASESEND`, `CASERECV`, `DEFAULT`
- **Slices**: `SLTYPE`, `SLMAKE`, `SLICE`
- **Structs**: `STTYPE`/`ENDSTTYPE`, `FIELD`, `FSET`, `FGET`
- **Maps**: `MPTYPE`, `MPMAKE`, `MSET`, `MGET`, `MPKEYS`
- **Closures**: `FNTYPE`, `CLOS`/`ENDCLOS`

Method calls (e.g. `file.Close()`) are expressed by declaring the method's function type with `FNTYPE`, then pulling the bound method value out of a struct value with `FGET`, and calling that value.

The **only authoritative specification is [`docs/amivm_spec.md`](docs/amivm_spec.md)**. If any other document (including this README) disagrees with it, `amivm_spec.md` wins. For a more readable, annotated walkthrough of the same spec (including the reasoning behind design decisions), see [`docs/amivm_instruction_spec.md`](docs/amivm_instruction_spec.md). For how the compiler itself is built internally (tokenizing, the `Kind`/`Category` system, AST assembly, the unused-variable self-healing pass, etc.), read the source under `cmd/amivm/` directly — it's commented throughout.

## Calling your own Go code

A downstream project built on AMIVM-IR (e.g. a language front end) will often want its generated programs to call into a runtime library it wrote itself, not just the Go standard library. This works through the same `?pkg.Func` / `CALL` mechanism already used for `fmt.Println` and friends — nothing IR-side is special-cased for "custom" code — but two things are worth knowing up front:

1. **Package your functions as an ordinary Go package**, in whatever module layout fits your project. It does not need to be published anywhere — a private module (fetched over a private VCS host with `GOPRIVATE`, or referenced with a `replace` directive for a local/monorepo setup) works exactly the same as a public one from amivm's point of view.
2. **The directory amivm writes its output into must be a Go module** (i.e. it or an ancestor has a `go.mod`). Type-checking a call into another package — whether a hand-written file sitting next to the generated one, or a separate package elsewhere in the same module — relies on `golang.org/x/tools/go/packages`, which (like `go build` itself) only looks beyond a single bare file when there's a module to resolve against.
3. **Pass `-i name=import-path` for any package amivm hasn't seen referenced before.** Auto-adding the right import for a bare identifier like `xxrt.Helper` is goimports' job, and it's only reliable for the standard library and packages already used somewhere — for a freshly-introduced package it can fail to add the import, or even guess the wrong path. Being explicit sidesteps the guess entirely.

For example, given a small runtime package:

```go
// xxrt/xxrt.go — package xxrt, part of (or a dependency of) the consuming module
package xxrt

func Helper(a, b int) int { return a*10 + b }
```

and this IR:

```
FUNC	!main	:
	VAR	%r	^int
	CALL	%r	:	?xxrt.Helper	1	2
	CALL	:	?fmt.Println	%r
	RET
ENDFUNC
```

running `amivm hello.ir -o hello.go -i xxrt=yourmodule/xxrt` (from inside `yourmodule`, or with `-o` pointing into it) generates a `hello.go` with `import xxrt "yourmodule/xxrt"` already in place, ready for `go build`.

**Calling methods** (e.g. `file.Close()`, or a method on your own type) works the same way regardless of whether the receiver type is a stdlib type or your own: declare the method's function type with `FNTYPE`, pull the bound method value out of a struct value with `FGET`, and call that value — see [`test_ir/16_method_call.ir`](test_ir/16_method_call.ir) for a worked example against `(*os.File).Close`.

## Constraints

- `FUNC` may only appear at the top level (no nested function definitions). `STTYPE` likewise cannot nest. `IF`, `LOOP`, `CLOS`, and `SEL` can all nest, including inside one another.
- Arrays are one-dimensional, fixed-length only. Multi-dimensional arrays are expected to be flattened by the (not-yet-written) front end before reaching AMIVM-IR.
- Semantic correctness (type checking, undefined identifiers, method existence, etc.) is entirely delegated to `go/types`; AMIVM itself only guarantees that it emits syntactically valid Go.

## Repository layout

```
cmd/amivm/
  token.go                   tokenizing + classifying tokens into Atoms (Kind system)
  astbuild.go                naming rules and Atom → ast.Expr assembly
  category.go                operand categories (allowed Kind sets) and validation
  parse_stmt.go              parsing of single-line instructions (VAR, SET, CALL, ...)
  parse_block.go             parsing of block constructs (FUNC/SEL/CLOS/STTYPE, TYPE decls)
  program.go                 top-level assembly (buildProgram)
  compile.go                 unused-variable self-healing + the Go source output pipeline
  main.go                    entry point (CLI arg parsing, main)
docs/
  amivm_spec.md               the authoritative specification (project overview + full IR reference)
  amivm_instruction_spec.md   annotated walkthrough of amivm_spec.md, with design rationale
Makefile                    build/test/clean tasks (`make help` for the full list)
test_ir/                    example IR programs, one file per instruction group
.github/workflows/test.yml  CI: gofmt/go vet/go test/make test on push and PR
CLAUDE.md                   project conventions for AI-assisted development
LICENSE                     MIT
```

## License

[MIT](LICENSE)
