package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

// skippedDirs は ForbiddenCalls が見ないディレクトリの名前。. で始まる名前も見ない。
var skippedDirs = []string{"testdata", "trash", "node_modules"}

// ForbiddenCalls は、root の下の本体の Go のコード（_test.go は除く）から、削除・ゴミ箱・
// 共有に関わる次の 5 つを探し、"相対パス:行: 説明" の一覧で返す。
//  1. セレクタ名が EmptyTrash
//  2. セレクタ名が Permissions
//  3. X.Files.Delete(...) の呼び出し
//  4. 複合リテラルのキーが Trashed
//  5. .Trashed への代入
//
// 文字列ではなく構文木の節で照合するので、このファイルの説明や文字列には反応しない。
func ForbiddenCalls(root string) ([]string, error) {
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
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("%s を解析できません: %w", p, err)
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			rel = p
		}
		rel = filepath.ToSlash(rel)
		ast.Inspect(file, func(n ast.Node) bool {
			if msg := forbiddenMessage(n); msg != "" {
				found = append(found, fmt.Sprintf("%s:%d: %s", rel, fset.Position(n.Pos()).Line, msg))
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// forbiddenMessage は、n が禁止する形なら説明を、そうでなければ "" を返す。
func forbiddenMessage(n ast.Node) string {
	switch n := n.(type) {
	case *ast.SelectorExpr:
		switch n.Sel.Name {
		case "EmptyTrash":
			return "EmptyTrash（ゴミ箱を空にする API）は使えません"
		case "Permissions":
			return "Permissions（共有の API）は使えません"
		}
	case *ast.CallExpr:
		if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Delete" {
			if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "Files" {
				return "Files.Delete（ファイルの削除）は使えません"
			}
		}
	case *ast.KeyValueExpr:
		if key, ok := n.Key.(*ast.Ident); ok && key.Name == "Trashed" {
			return "Trashed をキーにした複合リテラル（ゴミ箱への移動）は使えません"
		}
	case *ast.AssignStmt:
		for _, lhs := range n.Lhs {
			if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "Trashed" {
				return "Trashed への代入（ゴミ箱への移動）は使えません"
			}
		}
	}
	return ""
}
