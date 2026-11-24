// Package video は、動画のタイトル・説明・タグ・サムネイル・目印の決まりを持つ。
package video

import (
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

const (
	// PrivacyUnlisted は、このツールが使う唯一の公開範囲（限定公開）。
	PrivacyUnlisted = "unlisted"
	// MarkerTag は、このツールが上げた動画に付けるタグ。
	MarkerTag = "youtube-cli"
	// DefaultCategory は、カテゴリの既定値（22: ブログと人々）。
	DefaultCategory = "22"

	maxTitleRunes = 100
	maxDescBytes  = 5000
	maxTagsLength = 500
)

// Meta は、アップロードする動画の項目。
type Meta struct {
	Title          string
	Description    string
	Tags           []string
	CategoryID     string
	MadeForKids    bool
	SyntheticMedia bool
}

// ValidateMeta は、YouTube が受け付ける範囲に項目が収まっているかを確かめる。
func ValidateMeta(m Meta) error {
	if strings.TrimSpace(m.Title) == "" {
		return domain.Errorf(domain.KindUsage, "タイトルを指定してください")
	}
	if utf8.RuneCountInString(m.Title) > maxTitleRunes {
		return domain.Errorf(domain.KindUsage, "タイトルは %d 文字以内にしてください", maxTitleRunes)
	}
	if strings.ContainsAny(m.Title, "<>") {
		return domain.Errorf(domain.KindUsage, "タイトルに < と > は使えません")
	}
	if len(m.Description) > maxDescBytes {
		return domain.Errorf(domain.KindUsage, "説明は %d バイト以内にしてください", maxDescBytes)
	}
	// YouTube は < と > を含む説明を、動画を送り終えたあとで 400 で断る。送る前に止める。
	if strings.ContainsAny(m.Description, "<>") {
		return domain.Errorf(domain.KindUsage, "説明に < と > は使えません（-> や <- も使えません）")
	}
	if tagsLength(m.Tags) > maxTagsLength {
		return domain.Errorf(domain.KindUsage, "タグの合計は %d 文字以内にしてください", maxTagsLength)
	}
	return nil
}

// tagsLength は、YouTube の数え方でタグの合計の長さを返す。
// 空白を含むタグは前後の引用符 2 文字を足し、タグの間にカンマ 1 文字を足す。
func tagsLength(tags []string) int {
	n := 0
	for i, t := range tags {
		n += utf8.RuneCountInString(t)
		if strings.ContainsAny(t, " \t") {
			n += 2
		}
		if i > 0 {
			n++
		}
	}
	return n
}

// TitleFromPath は、ファイルのパスから拡張子とディレクトリを除いたタイトルを返す。
func TitleFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// WithMarker は、目印のタグを 1 つだけ含むタグの一覧を返す。
func WithMarker(tags []string) []string {
	if HasMarker(tags) {
		return slices.Clone(tags)
	}
	return append(slices.Clone(tags), MarkerTag)
}

// HasMarker は、目印のタグがあるかを返す。
func HasMarker(tags []string) bool { return slices.Contains(tags, MarkerTag) }
