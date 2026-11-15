//go:build darwin && cgo

package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"unicode/utf8"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/repository/keychain"
)

const msgNoHardenedRuntime = "この youtube-cli には hardened runtime が付いていないため、キーチェーンを使いません。`make install`（開発中は `make build`）で入れ直すか、`YOUTUBE_CLI_KEYRING=file` を指定してください"

// newPlatformOSKeyStore は、macOS では youtube-cli だけが読める新しい形の項目に鍵を置く KeyStore を返す（設計書 5.1・5.2）。
// go-keyring は、古い形の項目を移すときだけ使う。
func newPlatformOSKeyStore(dir string) KeyStore {
	return newNativeKeyStore(dir, secItems{}, goKeyringBackend{}, darwinGuard, func(msg string) { fmt.Fprintln(os.Stderr, msg) })
}

// darwinGuard は、キーチェーンに触れる前に1回だけ行う確かめ（設計書 5.3・5.4）。
// core ダンプを止めてから、自分の署名に hardened runtime が付いているかを見る。付いていない、または署名を読めなければ KindSetup。
var darwinGuard = sync.OnceValue(func() error {
	// 止められなくても続ける。止めるのは念のためで、キーチェーンを使えなくするほどではない。
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{Cur: 0, Max: 0})
	flags, status, st := keychain.SelfSignature()
	if st != keychain.StatusOK || !keychain.HardenedRuntime(flags, status) {
		return domain.Errorf(domain.KindSetup, msgNoHardenedRuntime)
	}
	return nil
})

var _ nativeItems = secItems{}

// secItems は、nativeItems を keychain の関数（Security.framework）で満たす。cgo との境目なので、
// 渡せないアカウント名は keychain を呼ぶ前に断る。
type secItems struct{}

func (secItems) Inspect(account string) (itemState, error) {
	if err := checkAccount(account); err != nil {
		return itemAbsent, err
	}
	comment, st := keychain.Comment(keyringService, account)
	switch {
	case st == keychain.StatusOK && comment == nativeMarker:
		return itemNative, nil
	case st == keychain.StatusOK:
		return itemForeign, nil
	case st == keychain.StatusItemNotFound:
		return itemAbsent, nil
	default:
		return itemAbsent, keychainErr(st)
	}
}

func (secItems) Get(account string) (string, error) {
	if err := checkAccount(account); err != nil {
		return "", err
	}
	b, st := keychain.Get(keyringService, account)
	defer clear(b)
	if err := keychainErr(st); err != nil {
		return "", err
	}
	return string(b), nil
}

func (secItems) Add(account, secret string) error {
	if err := checkAccount(account); err != nil {
		return err
	}
	b := []byte(secret)
	defer clear(b)
	return keychainErr(keychain.Add(keyringService, account, nativeLabel, nativeMarker, b))
}

func (secItems) Delete(account string) error {
	if err := checkAccount(account); err != nil {
		return err
	}
	return keychainErr(keychain.Delete(keyringService, account))
}

// checkAccount は、cgo に渡せないアカウント名を断る。UTF-8 でなければ CFString を作れずに落ち、
// NUL があれば C の文字列がそこで切れて、別の項目を指してしまうため。
func checkAccount(account string) error {
	if !utf8.ValidString(account) || strings.ContainsRune(account, 0) {
		return errors.New("キーチェーンのアカウント名（設定ディレクトリの絶対パス）に、UTF-8 でない文字か NUL が含まれています")
	}
	return nil
}
