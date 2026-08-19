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
	CatDeftype
	CatField
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
	CatVa: "VAR変数名", CatGv: "GVAR変数名",
	CatSingle: "単一左辺/チャネル変数", CatMulti: "複数左辺",
	CatVariable: "変数参照", CatType: "型", CatDeftype: "定義型",
	CatField: "構造体フィールド名",
	CatWhole: "0以上の整数", CatFromTo: "スライス範囲(from/to)",
	CatInt: "整数", CatNumber: "数値", CatBool: "真偽値",
	CatSlice: "スライス/文字列", CatOrder: "順序比較値", CatValue: "値",
	CatDefname: "関数定義名", CatCallname: "呼び出し対象", CatLabel: "ラベル名",
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
	m[CatDeftype] = kindSet(KType)
	m[CatField] = kindSet(KField)

	whole := mergeKinds(identRefFull, kindSet(KZero, KPosInt, KRune))
	m[CatWhole] = whole
	m[CatFromTo] = mergeKinds(whole, kindSet(KBlank))

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

	value := mergeKinds(order, kindSet(KBool, KNil))
	m[CatValue] = value

	m[CatDefname] = kindSet(KAmivmFunc, KAmivmMain)
	m[CatCallname] = kindSet(KAmivmFunc, KAmivmMain, KGoFunc, KGoFuncSel, KLocal)
	m[CatLabel] = kindSet(KLabel)

	return m
}

func atomExpr(a Atom, funcName string, cat Category) (ast.Expr, error) {
	if err := checkKind(a, cat); err != nil {
		return nil, err
	}
	return atomToExpr(a, funcName)
}

// checkKind はKindがカテゴリの許容集合に属するかだけを検証する。ast.Exprを組み立てない
// ため、$N/%xxxのようにfuncNameがないと組み立てられない形式や、KLabelのように
// atomToExprが扱わない(ラベルはlabelGoNameで別途名前を取り出すため)形式でも、
// 「検証だけしたい」箇所(VAR名・GVAR名・ラベル・関数定義名・deftype名の妥当性チェックなど)
// で安全に使える。
func checkKind(a Atom, cat Category) error {
	if a.Kind == KInvalid {
		return fmt.Errorf("%sとして解釈できない形式です: %s", categoryLabel[cat], a.Raw)
	}
	if !allowedKinds[cat][a.Kind] {
		return fmt.Errorf("%sにこの形式は使えません: %s", categoryLabel[cat], a.Raw)
	}
	return nil
}
