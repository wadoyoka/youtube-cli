package archtest

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// topLevel は internal/ の直下に置けるディレクトリ。
var topLevel = append(slices.Clone(layers), "archtest")

// CheckLayout は、internal/ の直下にある、置き場所の外のファイルとディレクトリを返す。
// 置き場所の外のものは層の外になり、CheckImport の表で確かめられない。
// cli・usecase・domain・repository・archtest のディレクトリだけが置ける。
//
// 名前の順に返す。. で始まる名前（Finder が作る .DS_Store など）は Go も読まないので見ない。
func CheckLayout(internalDir string) []string {
	var found []string
	entries, err := readDir(internalDir)
	if err != nil {
		found = append(found, unreadable(".", err))
	}
	for _, entry := range entries {
		if !entry.IsDir() || !slices.Contains(topLevel, entry.Name()) {
			found = append(found, entry.Name())
		}
	}
	slices.Sort(found)
	return found
}

// MissingTests は、_test.go の無い domain と usecase のパッケージのディレクトリを返す。
// .go のファイルが無いディレクトリはパッケージではないので見ない。テスト用の部品を置く
// パッケージ（testHelper）も見ない。
//
// パスは internalDir からの相対パス（区切りは "/"）で、名前の順に返す。Go が読まないもの
// （testdata と、. か _ で始まる名前）は見ない。
func MissingTests(internalDir string) []string {
	type goFiles struct{ code, test bool }
	pkgs := map[string]*goFiles{}
	var found []string

	// 読めなかったパスは found に入れて先に進むので、WalkDir が error を返すことは無い。
	_ = filepath.WalkDir(internalDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			found = append(found, unreadable(relSlash(internalDir, p), err))
			return nil
		}
		if d.IsDir() {
			if p != internalDir && ignoredByGo(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if ignoredByGo(name) || !strings.HasSuffix(name, ".go") {
			return nil
		}
		dir := relSlash(internalDir, filepath.Dir(p))
		if layer := classifyRel(path.Join("internal", dir)); layer != "domain" && layer != "usecase" {
			return nil
		}
		files, ok := pkgs[dir]
		if !ok {
			files = &goFiles{}
			pkgs[dir] = files
		}
		if strings.HasSuffix(name, "_test.go") {
			files.test = true
		} else {
			files.code = true
		}
		return nil
	})

	for dir, files := range pkgs {
		if files.code && !files.test && !testHelper(dir) {
			found = append(found, dir)
		}
	}
	slices.Sort(found)
	return found
}

// testHelper は、dir がテスト用の部品を置くパッケージ（標準ライブラリの httptest のように、
// 名前が test で終わるもの。usecase/usecasetest など）なら true を返す。
// ほかのパッケージのテストが使う偽物の置き場で、確かめる処理を持たないので、テストを求めない。
func testHelper(dir string) bool {
	return strings.HasSuffix(path.Base(dir), "test")
}

// readDir は dir の中身を、. で始まる名前を除いて返す。dir が無ければ（まだ internal/ が
// 無いときなど）空を返す。
func readDir(dir string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(entries, func(e fs.DirEntry) bool {
		return strings.HasPrefix(e.Name(), ".")
	}), nil
}

// ignoredByGo は、go build と go test が読まない名前なら true を返す。
func ignoredByGo(name string) bool {
	return name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// relSlash は internalDir からの相対パスを "/" 区切りで返す。
func relSlash(internalDir, p string) string {
	rel, err := filepath.Rel(internalDir, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

// unreadable は、読めなかったパスを、検査の結果に混ぜて返すための文面にする。
// 読めないまま何も返さないと、検査を通ったように見えてしまうため。
func unreadable(rel string, err error) string {
	return fmt.Sprintf("%s（読めません: %v）", rel, err)
}
