// Package keychain は、macOS の Security.framework を cgo で呼ぶ薄い層と、その結果の分類を持つ。
package keychain

// Status は Security.framework が返す OSStatus。
type Status int32

// 分類に使う OSStatus の値。
const (
	StatusOK                    Status = 0
	StatusUserCanceled          Status = -128
	StatusAuthFailed            Status = -25293
	StatusDuplicateItem         Status = -25299
	StatusItemNotFound          Status = -25300
	StatusInteractionNotAllowed Status = -25308
)

// Result は OSStatus を呼び出し側が扱う種類に分けたもの。
type Result int

// Result の種類。
const (
	ResultOK       Result = iota
	ResultNotFound        // 項目がない
	ResultDenied          // ダイアログで拒否された
	ResultNoUI            // ダイアログを出せない
	ResultOther           // それ以外（迷うものはここ）
)

// Classify は OSStatus を Result に分類する。
func Classify(st Status) Result {
	switch st {
	case StatusOK:
		return ResultOK
	case StatusItemNotFound:
		return ResultNotFound
	case StatusAuthFailed, StatusUserCanceled:
		return ResultDenied
	case StatusInteractionNotAllowed:
		return ResultNoUI
	default:
		return ResultOther
	}
}

// hardened runtime を表す 2 つの印。
const (
	// SignatureRuntime は kSecCodeSignatureRuntime（署名の印）。
	SignatureRuntime uint32 = 0x10000
	// CSRuntime は CS_RUNTIME（実行中の状態語）。
	CSRuntime uint32 = 0x10000
)

// HardenedRuntime は、署名の印と実行中の状態語の両方に hardened runtime が立つかを返す。
func HardenedRuntime(staticFlags, dynamicStatus uint32) bool {
	return staticFlags&SignatureRuntime != 0 && dynamicStatus&CSRuntime != 0
}
