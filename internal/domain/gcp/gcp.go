// Package gcp は、setup が扱う GCP の値（プロジェクト ID の形、コンソールの URL）の決まりを持つ。
package gcp

import (
	"net/url"
	"regexp"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

var projectIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)

// ValidateProjectID は、GCP のプロジェクト ID の形を確かめる。
// 小文字で始まり、小文字・数字・ハイフンだけで 6〜30 文字、最後は小文字か数字。合わなければ KindUsage。
func ValidateProjectID(id string) error {
	if !projectIDPattern.MatchString(id) {
		return domain.Errorf(domain.KindUsage, "プロジェクト ID %q が正しくありません。小文字で始まり、小文字・数字・ハイフンだけで 6〜30 文字、最後は小文字か数字にしてください", id)
	}
	return nil
}

// AuthConsoleURL は、Google Auth Platform の画面の URL を返す。
func AuthConsoleURL(page, projectID string) string {
	return "https://console.cloud.google.com/auth/" + page + "?project=" + url.QueryEscape(projectID)
}
