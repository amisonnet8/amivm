package main

import (
	"fmt"
	"go/ast"
)

// =====================================================================
// オペランドカテゴリ(第3層)
// =====================================================================

type Category int

const (
	CatVa Category = iota
	CatGv
	CatSingle
	CatMulti
	CatVariable
	CatType
	CatTypename
	CatConstraint
	CatReceiver
	CatField
	CatMethod
	CatPoint
	CatImm
	CatWhole
	CatFromTo
	CatInt
	CatNumber
	CatBool
	CatSlice
	CatOrder
	CatValue
	CatDefname
	CatCallname
	CatLabel
)

var categoryLabel = map[Category]string{
	CatVa: "VAR variable name", CatGv: "GVAR variable name",
	CatSingle: "single left-hand side/channel variable", CatMulti: "multiple left-hand side",
	CatVariable: "variable reference", CatType: "type", CatTypename: "defined type/type parameter name",
	CatConstraint: "type parameter constraint", CatReceiver: "FUNCM receiver type",
	CatField: "struct field name", CatMethod: "method name", CatPoint: "ADDR field/index target",
	CatImm:   "immediate value (compile-time constant literal)",
	CatWhole: "non-negative integer", CatFromTo: "slice range (from/to)",
	CatInt: "integer", CatNumber: "number", CatBool: "boolean",
	CatSlice: "slice/string", CatOrder: "orderable comparison value", CatValue: "value",
	CatDefname: "function definition name", CatCallname: "call target", CatLabel: "label name",
}

func kindSet(kinds ...Kind) map[Kind]bool {
	m := make(map[Kind]bool, len(kinds))
	for _, k := range kinds {
		m[k] = true
	}
	return m
}

func mergeKinds(sets ...map[Kind]bool) map[Kind]bool {
	m := map[Kind]bool{}
	for _, s := range sets {
		for k := range s {
			m[k] = true
		}
	}
	return m
}

var (
	// identRefFull: $ / & / % / @(プレーン・セレクタ)の単純参照。
	identRefFull = kindSet(KParam, KClosureParam, KLocal, KGlobal, KGlobalSel)
	// identRefNoSel: identRefFullから@xxx.yyy(他パッケージ参照)を除いたもの。
	// 「single」「multi」カテゴリで使う。
	identRefNoSel = kindSet(KParam, KClosureParam, KLocal, KGlobal)

	typeKindsAll = kindSet(
		KType, KTypeSel, KTypePtr, KTypePtrSel,
		KArrType, KArrTypeSel, KArrTypePtr, KArrTypePtrSel,
	)
)

var allowedKinds = buildAllowedKinds()

func buildAllowedKinds() map[Category]map[Kind]bool {
	m := map[Category]map[Kind]bool{}

	m[CatVa] = kindSet(KLocal)
	m[CatGv] = kindSet(KGlobal)
	m[CatSingle] = identRefNoSel
	m[CatMulti] = mergeKinds(identRefNoSel, kindSet(KBlank))
	m[CatVariable] = identRefFull

	m[CatType] = typeKindsAll
	m[CatTypename] = kindSet(KType)
	m[CatConstraint] = kindSet(KType, KTypeSel)
	m[CatReceiver] = kindSet(KType, KTypePtr)
	m[CatField] = kindSet(KField)
	m[CatMethod] = kindSet(KMethod)

	// imm: コンパイル時定数のリテラルのみ(識別子は不可)。ARTYPEの配列サイズのように、
	// Goの型レベルで定数式が要求される箇所(変数を許すと`go/types`ではなく構文の
	// 時点で妥当性が崩れる)で使う。
	imm := kindSet(KZero, KPosInt, KRune)
	m[CatImm] = imm

	whole := mergeKinds(identRefFull, imm)
	m[CatWhole] = whole
	m[CatFromTo] = mergeKinds(whole, kindSet(KBlank))
	// point: ADDRの第3引数(フィールド/添字の対象)。wholeの許容形式に構造体フィールド名を加えたもの。
	m[CatPoint] = mergeKinds(whole, kindSet(KField))

	integer := mergeKinds(whole, kindSet(KNegInt))
	m[CatInt] = integer

	number := mergeKinds(integer, kindSet(KFloat))
	m[CatNumber] = number

	boolean := mergeKinds(identRefFull, kindSet(KBool))
	m[CatBool] = boolean

	slice := mergeKinds(identRefFull, kindSet(KString))
	m[CatSlice] = slice

	order := mergeKinds(identRefFull, kindSet(KZero, KPosInt, KNegInt, KFloat, KRune, KString))
	m[CatOrder] = order

	// value: 値全般。KAmivmFunc/KGoFunc/KGoFuncSelを含めることで、関数そのものを
	// (呼び出さずに)値として引数等に渡せる。KAmivmMainは含めない(mainを値として渡す
	// ケースは無い)。
	value := mergeKinds(order, kindSet(KBool, KNil, KAmivmFunc, KGoFunc, KGoFuncSel))
	m[CatValue] = value

	m[CatDefname] = kindSet(KAmivmFunc, KAmivmMain)
	// callname: 呼び出し対象。KParam/KClosureParamを含めることで、パラメータ・
	// クロージャー引数として受け取った関数値(クロージャー等)をそのまま呼び出せる。
	// KGlobalを含めることで、パッケージレベル変数に格納された関数値もそのまま呼び出せる。
	m[CatCallname] = kindSet(KAmivmFunc, KAmivmMain, KGoFunc, KGoFuncSel, KLocal, KGlobal, KParam, KClosureParam)
	m[CatLabel] = kindSet(KLabel)

	return m
}

func atomExpr(a Atom, funcName string, closureLevel int, cat Category) (ast.Expr, error) {
	if err := checkKind(a, cat); err != nil {
		return nil, err
	}
	return atomToExpr(a, funcName, closureLevel)
}

// checkKind はKindがカテゴリの許容集合に属するかだけを検証する。ast.Exprを組み立てない
// ため、$N/%xxxのようにfuncNameがないと組み立てられない形式や、KLabelのように
// atomToExprが扱わない(ラベルはlabelGoNameで別途名前を取り出すため)形式でも、
// 「検証だけしたい」箇所(VAR名・GVAR名・ラベル・関数定義名・deftype名の妥当性チェックなど)
// で安全に使える。
func checkKind(a Atom, cat Category) error {
	if a.Kind == KInvalid {
		return fmt.Errorf("unparseable format for %s: %s", categoryLabel[cat], a.Raw)
	}
	if !allowedKinds[cat][a.Kind] {
		return fmt.Errorf("this format cannot be used for %s: %s", categoryLabel[cat], a.Raw)
	}
	// $0(FUNCMのレシーバー)は$Nが許容される全カテゴリで使えるが、single/multi
	// (代入先・左辺値)だけは例外。レシーバー自体への再代入は認めない設計とするため、
	// go/typesには委ねずここで構文的に弾く(Goの構文としては$0=...も合法なため)。
	if a.Kind == KParam && a.A == "0" && (cat == CatSingle || cat == CatMulti) {
		return fmt.Errorf("$0 (receiver) cannot be used for %s: %s", categoryLabel[cat], a.Raw)
	}
	return nil
}
