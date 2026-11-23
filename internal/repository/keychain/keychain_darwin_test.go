//go:build darwin && cgo

package keychain

import (
	"runtime"
	"testing"
)

func TestSelfSignature(t *testing.T) {
	t.Run("go test の実行ファイルには hardened runtime が付いていない", func(t *testing.T) {
		// Arrange
		// 自分の署名を読むだけで、キーチェーンには触れない。

		// Act
		s, d, st := SelfSignature()

		// Assert
		if runtime.GOARCH == "arm64" && st != StatusOK {
			t.Fatalf("SelfSignature() の st = %d, arm64 では ad-hoc 署名が付くので %d のはず", st, StatusOK)
		}
		if st == StatusOK && HardenedRuntime(s, d) {
			t.Errorf("HardenedRuntime(%#x, %#x) = true, go test の実行ファイルには付いていないはず", s, d)
		}
	})
}
