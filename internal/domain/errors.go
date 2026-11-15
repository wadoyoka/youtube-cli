// Package domain は、どの層からも使われる純粋な中核（エラー、動画や再生リストの項目など）を持つ。
// 個別の決まりは、サブパッケージ（policy・gcp・configdir・winpath）に置く。
package domain

import (
	"errors"
	"fmt"
)

// ErrorKind は、エラーの種類。終了コードに対応する。
type ErrorKind int

const (
	// KindUnexpected は、予期しないエラー（終了コード 1）。
	KindUnexpected ErrorKind = iota
	// KindUsage は、引数の誤り（終了コード 2）。
	KindUsage
	// KindSetup は、準備が必要（終了コード 3）。
	KindSetup
	// KindNotFound は、見つからない（終了コード 4）。
	KindNotFound
	// KindConflict は、衝突（終了コード 5）。
	KindConflict
	// KindAmbiguous は、あいまい（終了コード 6）。
	KindAmbiguous
	// KindDenied は、拒否された（終了コード 7）。
	KindDenied
	// KindQuota は、YouTube API の割り当て（クォータ）を使い切った（終了コード 8）。
	KindQuota
)

var kindNames = [...]string{
	KindUnexpected: "unexpected",
	KindUsage:      "usage",
	KindSetup:      "setup",
	KindNotFound:   "not_found",
	KindConflict:   "conflict",
	KindAmbiguous:  "ambiguous",
	KindDenied:     "denied",
	KindQuota:      "quota",
}

// ExitCode は、この種類に対応するプロセスの終了コードを返す。
func (k ErrorKind) ExitCode() int {
	if k < KindUnexpected || int(k) >= len(kindNames) {
		return 1
	}
	return int(k) + 1
}

// String は、この種類の名前を返す。
func (k ErrorKind) String() string {
	if k < KindUnexpected || int(k) >= len(kindNames) {
		return kindNames[KindUnexpected]
	}
	return kindNames[k]
}

// Error は、種類つきのエラー。
type Error struct {
	Kind    ErrorKind
	Message string
	Details []string
	Err     error
}

// Error は、Err があれば "Message: Err" を、なければ Message を返す。
func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Unwrap は、原因のエラーを返す。
func (e *Error) Unwrap() error { return e.Err }

// Errorf は、種類と書式から Error を作る。
func Errorf(kind ErrorKind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// WithDetails は、補足を足した Error を返す。
func (e *Error) WithDetails(details ...string) *Error {
	e.Details = append(e.Details, details...)
	return e
}

// Wrap は、原因を付けた Error を返す。
func (e *Error) Wrap(err error) *Error {
	e.Err = err
	return e
}

// KindOf は、err の連なりから *Error を探し、その種類を返す。なければ KindUnexpected。
func KindOf(err error) ErrorKind {
	if de, ok := errors.AsType[*Error](err); ok {
		return de.Kind
	}
	return KindUnexpected
}
