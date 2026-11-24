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

// Is は、target が種類と本文の同じ Error なら true を返す。e か target が nil の *Error なら false。
// 決まったエラー（ErrUploadUncertain など）に原因を付けた写しを、errors.Is で元のエラーと見分けられるようにするため。
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && e != nil && t != nil && t.Kind == e.Kind && t.Message == e.Message
}

// ErrUploadUncertain は、動画を送り始めたあとで動画 ID を受け取れず、YouTube に動画ができたか分からないこと。
// 二重に上げないよう、送り直さずに Studio で確かめてもらう。結果が分からない動画は手元の記録にないので、
// --skip-existing が見つけられるのは YouTube の一覧（ls）に出てから。出る前に付けると、もう 1 本上がる。
// 原因を付けるときは UploadUncertain を使う（これ自体は変えない）。
var ErrUploadUncertain = Errorf(KindUnexpected, "アップロードの結果が分かりません。Studio に動画があれば、`youtube-cli ls --json` に出るまで待ってから、同じコマンドに `--skip-existing` を付けてやり直してください（出る前に付けると、もう1本上がります）")

// UploadUncertain は、原因 cause を付けた ErrUploadUncertain の写しを返す。
func UploadUncertain(cause error) *Error {
	return &Error{Kind: ErrUploadUncertain.Kind, Message: ErrUploadUncertain.Message, Err: cause}
}

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
