package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// skippedDirs は ForbiddenCalls が見ないディレクトリの名前。. で始まる名前も見ない。
var skippedDirs = []string{"testdata", "trash", "node_modules"}

const (
	// youtubeAPIPath は YouTube Data API のクライアントの import パス。
	youtubeAPIPath = "google.golang.org/api/youtube/v3"
	// domainVideoPath は、限定公開の定数 PrivacyUnlisted がある domain/video の import パス。
	domainVideoPath = "github.com/wadoyoka/youtube-cli/internal/domain/video"
)

// forbiddenServiceMethods は、呼ぶと動画・再生リスト・チャンネルを消す・書き換える API。
// キーは Service のフィールド名、値は禁止するメソッド名（nil なら、フィールドを使うこと自体を禁止する）。
var forbiddenServiceMethods = map[string][]string{
	"Videos":         {"Delete", "Update"},
	"Playlists":      {"Delete", "Update"},
	"PlaylistItems":  {"Delete", "Update"},
	"Channels":       {"Update"},
	"Comments":       nil,
	"CommentThreads": nil,
	"Captions":       nil,
	"ChannelBanners": nil,
	"Watermarks":     nil,
	"LiveBroadcasts": nil,
	"LiveStreams":    nil,
}

// ForbiddenCalls は、root の下の本体の Go のコード（_test.go は除く）から、削除・更新・
// 公開範囲の変更に関わる次の形を探し、"相対パス:行: 説明" の一覧で返す。
//  1. forbiddenServiceMethods にある API の使用（Videos.Delete / Videos.Update、
//     Playlists.Delete / Playlists.Update、PlaylistItems.Delete / PlaylistItems.Update、
//     Channels.Update、Comments、CommentThreads、Captions、ChannelBanners、Watermarks、
//     LiveBroadcasts、LiveStreams）
//  2. YouTube へ送る公開範囲に、video.PrivacyUnlisted 以外を入れる形。対象は次の 3 つ。
//     - youtube.VideoStatus / youtube.PlaylistStatus の複合リテラルの PrivacyStatus キー
//     - それらの型の変数への <x>.PrivacyStatus の代入
//     - <x>.Status.PrivacyStatus への代入
//
// 文字列ではなく構文木の節で照合するので、このファイルの説明や文字列には反応しない。
// 2 の「youtube の型」と「video.PrivacyUnlisted」は、ファイルの import から別名も含めて決める。
//
// 公開範囲の検査は構文だけで行う（型の情報は読み込まない）ので、次の抜けが残る。
//   - 変数の型を、同じファイルの中の宣言（:= の複合リテラル、var、関数の引数と戻り値）からしか
//     知れない。他のファイルや関数の戻り値から来た youtube.VideoStatus の変数への代入は見えない
//     （ただし Video.Status.PrivacyStatus のように .Status 経由の代入は見える）。
//   - 変数の名前だけを覚えるので、同じファイルで同じ名前を別の型にも使うと、読み取りの代入を
//     誤って禁止することがある（その場合は変数の名前を変える）。
//   - PrivacyUnlisted を別の変数に入れ直してから渡す形は、値が分からないので禁止になる。
//
// 応答を読んで domain や cli の構造体へ写す代入と複合リテラルは、YouTube へ送るものではないので対象外。
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
		c := newFileChecker(file)
		ast.Inspect(file, func(n ast.Node) bool {
			if at, msg := c.message(n); msg != "" {
				found = append(found, fmt.Sprintf("%s:%d: %s", rel, fset.Position(at.Pos()).Line, msg))
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

// fileChecker は、1 つのファイルの import と変数の宣言を覚えて、禁止する形かを調べる。
type fileChecker struct {
	youtubeNames map[string]bool // youtube/v3 を指す import の名前
	videoNames   map[string]bool // domain/video を指す import の名前
	statusVars   map[string]bool // youtube.VideoStatus / PlaylistStatus（のポインタ）の変数の名前
}

func newFileChecker(file *ast.File) *fileChecker {
	c := &fileChecker{youtubeNames: map[string]bool{}, videoNames: map[string]bool{}, statusVars: map[string]bool{}}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := ""
		if imp.Name != nil {
			name = imp.Name.Name
		}
		switch path {
		case youtubeAPIPath:
			if name == "" {
				name = "youtube" // v3 の package 名
			}
			c.youtubeNames[name] = true
		case domainVideoPath:
			if name == "" {
				name = "video"
			}
			c.videoNames[name] = true
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE && len(n.Lhs) == len(n.Rhs) {
				for i, lhs := range n.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && c.isStatusValue(n.Rhs[i]) {
						c.statusVars[id.Name] = true
					}
				}
			}
		case *ast.ValueSpec:
			for i, id := range n.Names {
				if c.isStatusType(n.Type) || (i < len(n.Values) && c.isStatusValue(n.Values[i])) {
					c.statusVars[id.Name] = true
				}
			}
		case *ast.Field: // 関数の引数・戻り値・構造体のフィールド
			if c.isStatusType(n.Type) {
				for _, id := range n.Names {
					c.statusVars[id.Name] = true
				}
			}
		}
		return true
	})
	return c
}

// isStatusType は、expr が youtube.VideoStatus・youtube.PlaylistStatus（かそのポインタ）という型かを返す。
func (c *fileChecker) isStatusType(expr ast.Expr) bool {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && c.youtubeNames[pkg.Name] && (sel.Sel.Name == "VideoStatus" || sel.Sel.Name == "PlaylistStatus")
}

// isStatusValue は、expr が youtube.VideoStatus{...}・&youtube.PlaylistStatus{...} などの複合リテラルかを返す。
func (c *fileChecker) isStatusValue(expr ast.Expr) bool {
	if u, ok := expr.(*ast.UnaryExpr); ok && u.Op == token.AND {
		expr = u.X
	}
	lit, ok := expr.(*ast.CompositeLit)
	return ok && lit.Type != nil && c.isStatusType(lit.Type)
}

// isUnlisted は、expr がちょうど video.PrivacyUnlisted（domain/video の PrivacyUnlisted）かを返す。
func (c *fileChecker) isUnlisted(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "PrivacyUnlisted" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && c.videoNames[pkg.Name]
}

// message は、n が禁止する形なら、その場所の節と説明を、そうでなければ "" を返す。
func (c *fileChecker) message(n ast.Node) (ast.Node, string) {
	switch n := n.(type) {
	case *ast.SelectorExpr:
		// X.Videos.Delete や X.Comments のように、Service のフィールドを経由した API の使用。
		if methods, ok := forbiddenServiceMethods[n.Sel.Name]; ok && methods == nil {
			return n, n.Sel.Name + "（動画・コメント・チャンネルの更新や削除にあたる API）は使えません"
		}
		if inner, ok := n.X.(*ast.SelectorExpr); ok && slices.Contains(forbiddenServiceMethods[inner.Sel.Name], n.Sel.Name) {
			return n, inner.Sel.Name + "." + n.Sel.Name + "（削除・更新の API）は使えません"
		}
	case *ast.CompositeLit:
		if !c.isStatusType(n.Type) {
			break
		}
		for _, elt := range n.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "PrivacyStatus" && !c.isUnlisted(kv.Value) {
				return kv, "YouTube へ送る PrivacyStatus に video.PrivacyUnlisted 以外を入れられません"
			}
		}
	case *ast.AssignStmt:
		for i, lhs := range n.Lhs {
			if !c.isPrivacyTarget(lhs) {
				continue
			}
			if len(n.Lhs) == len(n.Rhs) && n.Tok == token.ASSIGN && c.isUnlisted(n.Rhs[i]) {
				continue
			}
			return n, "YouTube へ送る PrivacyStatus へ video.PrivacyUnlisted 以外を代入できません"
		}
	}
	return n, ""
}

// isPrivacyTarget は、lhs が YouTube へ送る PrivacyStatus かを返す。
// <x>.Status.PrivacyStatus か、statusVars の変数の <x>.PrivacyStatus。
func (c *fileChecker) isPrivacyTarget(lhs ast.Expr) bool {
	sel, ok := lhs.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "PrivacyStatus" {
		return false
	}
	switch x := sel.X.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "Status"
	case *ast.Ident:
		return c.statusVars[x.Name]
	}
	return false
}
