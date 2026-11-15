//go:build darwin && !cgo

package auth

import (
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

func TestPlatformKeyStore(t *testing.T) {
	t.Run("cgo なしの macOS では、OS のキーチェーンは使えないと返し、go-keyring に戻らない", func(t *testing.T) {
		// Arrange
		// メソッドを呼ぶ前に型を確かめる。組み立てを誤っていたときに、本物の go-keyring へ届かないようにするため。
		got := NewKeyStore(t.TempDir(), "os")
		s, ok := got.(unavailableKeyStore)
		if !ok {
			t.Fatalf("NewKeyStore(dir, \"os\") = %T, unavailableKeyStore のはず", got)
		}
		k, _ := newKey()

		// Act
		_, found, loadErr := s.Load()
		_, foundLocked, loadLockedErr := s.LoadLocked()
		saveErr := s.Save(k)
		adoptErr := s.Adopt(k)

		// Assert
		if s.Kind() != "os" {
			t.Fatalf("Kind() = %q, os のはず", s.Kind())
		}
		if found || foundLocked {
			t.Fatalf("found = %v, LoadLocked の found = %v, どちらも false のはず", found, foundLocked)
		}
		for name, err := range map[string]error{"Load": loadErr, "LoadLocked": loadLockedErr, "Save": saveErr, "Adopt": adoptErr} {
			if domain.KindOf(err) != domain.KindSetup || !strings.Contains(err.Error(), "cgo") {
				t.Fatalf("%s() = %v, cgo の文言の KindSetup のはず", name, err)
			}
		}
	})
}
