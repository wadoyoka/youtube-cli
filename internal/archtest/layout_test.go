package archtest

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCheckLayout(t *testing.T) {
	tests := []struct {
		name        string
		internalDir string
		want        []string
	}{
		{"internal の直下にある、置き場所の外のディレクトリとファイルを返す", "testdata/layout-bad/internal", []string{"misc.go", "util"}},
		{"直下が層と archtest のディレクトリだけなら何も返さない", "testdata/layout-good/internal", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			internalDir := tt.internalDir

			// Act
			got := CheckLayout(internalDir)

			// Assert
			if !slices.Equal(got, tt.want) {
				t.Errorf("CheckLayout(%q) = %q, 期待は %q", internalDir, got, tt.want)
			}
		})
	}
}

func TestCheckLayout_隠しファイルは見ない(t *testing.T) {
	// Arrange
	// .DS_Store は Finder が勝手に作り、Go も . で始まる名前を読まないので、層の外として扱わない。
	internalDir := filepath.Join(t.TempDir(), "internal")
	writeFile(t, filepath.Join(internalDir, ".DS_Store"))
	writeFile(t, filepath.Join(internalDir, "domain", "d.go"))

	// Act
	got := CheckLayout(internalDir)

	// Assert
	if len(got) != 0 {
		t.Errorf("CheckLayout は何も返さないはずが %q を返しました", got)
	}
}

func TestMissingTests(t *testing.T) {
	tests := []struct {
		name        string
		internalDir string
		want        []string
	}{
		{"_test.go の無い domain のパッケージだけを返す", "testdata/tests-missing/internal", []string{"domain"}},
		{"domain と usecase（サブパッケージを含む）にテストがあれば何も返さない。名前が test で終わるテスト用の部品のパッケージには求めない", "testdata/tests-ok/internal", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			internalDir := tt.internalDir

			// Act
			got := MissingTests(internalDir)

			// Assert
			if !slices.Equal(got, tt.want) {
				t.Errorf("MissingTests(%q) = %q, 期待は %q", internalDir, got, tt.want)
			}
		})
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}
