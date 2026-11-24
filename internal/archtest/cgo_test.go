package archtest

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCgoFiles(t *testing.T) {
	t.Run("keychain の外で import \"C\" を持つファイルだけを返す", func(t *testing.T) {
		// Arrange
		root := t.TempDir()
		withC := "package p\n\nimport \"C\"\n"
		withoutC := "package p\n"
		files := map[string]string{
			"a/x.go":                            withC,
			"internal/repository/keychain/y.go": withC,
			"b/z.go":                            withoutC,
			"trash/t.go":                        withC,
		}
		for rel, src := range files {
			p := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
				t.Fatal(err)
			}
		}

		// Act
		got, err := CgoFiles(root)
		// Assert
		if err != nil {
			t.Fatalf("CgoFiles(%q) がエラーを返しました: %v", root, err)
		}
		if want := []string{"a/x.go"}; !slices.Equal(got, want) {
			t.Errorf("CgoFiles(%q) = %q, 期待は %q", root, got, want)
		}
	})
}
