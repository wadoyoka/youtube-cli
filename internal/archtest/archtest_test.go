package archtest

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// このリポジトリ自身が、層の構成と依存のルールに従っているかを確かめる。
func TestRepositoryFollowsArchitecture(t *testing.T) {
	t.Run("internal の import が層の依存のルールに従っている", func(t *testing.T) {
		// Arrange
		// テストのファイルは、組み立てた部品をまとめて動かすために何でも読むので、見ない。
		cfg := &packages.Config{
			Mode: packages.NeedName | packages.NeedImports,
			Dir:  "../..",
		}
		pkgs, err := packages.Load(cfg, "./...")
		if err != nil {
			t.Fatalf("パッケージを読み込めません: %v", err)
		}

		// Act
		var checked int
		var violations []string
		for _, pkg := range pkgs {
			if !strings.HasPrefix(pkg.PkgPath, modulePath+"/internal/") {
				continue
			}
			for _, e := range pkg.Errors {
				t.Errorf("%s を読み込めません: %v", pkg.PkgPath, e)
			}
			checked++
			from := Classify(modulePath, pkg.PkgPath)
			for _, imported := range slices.Sorted(maps.Keys(pkg.Imports)) {
				if err := CheckImport(from, Classify(modulePath, imported)); err != nil {
					violations = append(violations, fmt.Sprintf("%s → %s: %v", inModule(pkg.PkgPath), inModule(imported), err))
				}
			}
		}

		// Assert
		if checked == 0 {
			t.Fatal("internal/ のパッケージを 1 つも読み込めていません")
		}
		if len(violations) > 0 {
			t.Errorf("層の依存のルールに反する import があります:\n%s", strings.Join(violations, "\n"))
		}
	})

	t.Run("internal の中のものが決まった置き場所にある", func(t *testing.T) {
		// Arrange
		internalDir := "../../internal"

		// Act
		got := CheckLayout(internalDir)

		// Assert
		if len(got) > 0 {
			t.Errorf("次のものが置き場所の外にあります。internal/ の直下は cli・usecase・domain・repository・archtest のディレクトリだけにしてください:\n%s", inInternal(got))
		}
	})

	t.Run("domain と usecase のパッケージにテストがある", func(t *testing.T) {
		// Arrange
		internalDir := "../../internal"

		// Act
		got := MissingTests(internalDir)

		// Assert
		if len(got) > 0 {
			t.Errorf("次のパッケージに _test.go がありません:\n%s", inInternal(got))
		}
	})

	t.Run("本体のコードが削除と共有の API を呼んでいない", func(t *testing.T) {
		// Arrange
		root := "../.."

		// Act
		got, err := ForbiddenCalls(root)
		// Assert
		if err != nil {
			t.Fatalf("ForbiddenCalls(%q) がエラーを返しました: %v", root, err)
		}
		if len(got) > 0 {
			t.Errorf("本体のコードに、削除・ゴミ箱・共有の API の呼び出しがあります:\n%s", strings.Join(got, "\n"))
		}
	})
}

func TestCgoOnlyInKeychain(t *testing.T) {
	// Arrange
	root := "../.."

	// Act
	got, err := CgoFiles(root)
	// Assert
	if err != nil {
		t.Fatalf("CgoFiles(%q) がエラーを返しました: %v", root, err)
	}
	if len(got) > 0 {
		t.Errorf("cgo（import \"C\"）は internal/repository/keychain/ の中だけに置けます。次のファイルが外にあります:\n%s", strings.Join(got, "\n"))
	}
}

// inModule は、この module のパッケージのパスを module の直下からのパスにする。
func inModule(pkgPath string) string {
	return strings.TrimPrefix(pkgPath, modulePath+"/")
}

// inInternal は internal/ からの相対パスを、リポジトリの直下からのパスにして 1 行ずつ並べる。
func inInternal(rels []string) string {
	lines := make([]string, 0, len(rels))
	for _, rel := range rels {
		lines = append(lines, "internal/"+rel)
	}
	return strings.Join(lines, "\n")
}
