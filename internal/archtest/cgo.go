package archtest

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

// cgoAllowedDir は、cgo（import "C"）を置いてよいディレクトリ（root からの相対パス）。
const cgoAllowedDir = "internal/repository/keychain/"

// CgoFiles は、root の下の .go ファイル（_test.go も含む）のうち、import "C" を持ち、
// internal/repository/keychain/ の外にあるものを、root からの相対パス（スラッシュ区切り）で返す。
// 見ないディレクトリは ForbiddenCalls と同じ（skippedDirs と、. で始まる名前）。
func CgoFiles(root string) ([]string, error) {
	var found []string
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && (strings.HasPrefix(d.Name(), ".") || slices.Contains(skippedDirs, d.Name())) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			rel = p
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, cgoAllowedDir) {
			return nil
		}
		file, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly|parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("%s を解析できません: %w", p, err)
		}
		for _, imp := range file.Imports {
			if imp.Path.Value == `"C"` {
				found = append(found, rel)
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}
