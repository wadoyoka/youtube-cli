//go:build darwin && cgo

package auth

import (
	"errors"
	"strings"
	"syscall"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

// このファイルのテストは、本物のキーチェーンに触れない。darwinGuard は自分の署名を読むだけで、
// secItems に渡すアカウント名は、キーチェーンを呼ぶ前に断られるものだけにする。
func TestPlatformKeyStore(t *testing.T) {
	t.Run("OS のキーチェーンは新しい形の KeyStore になり、go-keyring には戻らない", func(t *testing.T) {
		for _, mode := range []string{"os", ""} {
			// Arrange
			dir := t.TempDir()

			// Act
			s := NewKeyStore(dir, mode)

			// Assert
			// 型だけを見る。この KeyStore の Load・LoadLocked・Save・Adopt は本物のキーチェーンに届きうるので呼ばない。
			ns, ok := s.(*nativeKeyStore)
			if !ok {
				t.Fatalf("NewKeyStore(dir, %q) = %T, *nativeKeyStore のはず", mode, s)
			}
			if _, ok := ns.items.(secItems); !ok {
				t.Fatalf("NewKeyStore(dir, %q) の items = %T, secItems のはず", mode, ns.items)
			}
			// guard が配線されていることを、hardened runtime のない go test の実行ファイルで確かめる。
			gerr := ns.guard()
			if domain.KindOf(gerr) != domain.KindSetup || gerr == nil || !strings.Contains(gerr.Error(), "hardened runtime") {
				t.Fatalf("NewKeyStore(dir, %q) の guard() = %v, hardened runtime の KindSetup のはず", mode, gerr)
			}
		}
	})

	t.Run("go test の実行ファイルでは、guard が hardened runtime の文言で止まり、core ダンプの上限が 0 になる", func(t *testing.T) {
		// Arrange
		// go test の実行ファイルには hardened runtime が付いていない。guard は自分の署名を読むだけで、キーチェーンには触れない。
		var r syscall.Rlimit

		// Act
		err := darwinGuard()
		rlErr := syscall.Getrlimit(syscall.RLIMIT_CORE, &r)

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Fatalf("darwinGuard() = %v, KindSetup のはず", err)
		}
		if !strings.Contains(err.Error(), "hardened runtime") {
			t.Fatalf("darwinGuard() = %v, hardened runtime の文言のはず", err)
		}
		if rlErr != nil {
			t.Fatalf("Getrlimit: %v", rlErr)
		}
		if r.Cur != 0 || r.Max != 0 {
			t.Fatalf("RLIMIT_CORE = {Cur: %d, Max: %d}, どちらも 0 のはず", r.Cur, r.Max)
		}
	})

	t.Run("アカウント名が UTF-8 でないか NUL を含むときは、キーチェーンを呼ばずにエラーを返す", func(t *testing.T) {
		for _, account := range []string{"/tmp/youtube-cli-\xff", "/tmp/youtube-cli\x00/other"} {
			// Arrange
			// checkAccount が断らなければ、下の呼び出しは本物のキーチェーンに届くので、呼ぶ前に止める。
			if checkAccount(account) == nil {
				t.Fatalf("checkAccount(%q) = nil, 断るはず", account)
			}
			var items secItems

			// Act
			_, inspectErr := items.Inspect(account)
			_, getErr := items.Get(account)
			addErr := items.Add(account, "dummy-secret")
			deleteErr := items.Delete(account)

			// Assert
			for name, err := range map[string]error{"Inspect": inspectErr, "Get": getErr, "Add": addErr, "Delete": deleteErr} {
				// 「ない」と取り違えると、新しい鍵を作ったり、消えたものとみなしたりするので、errKeyringNotFound でもいけない。
				if err == nil || errors.Is(err, errKeyringNotFound) {
					t.Fatalf("%s(%q) = %v, 「ない」ではないエラーのはず", name, account, err)
				}
			}
		}
	})
}
