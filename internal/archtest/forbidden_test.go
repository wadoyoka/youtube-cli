package archtest

import (
	"fmt"
	"strings"
	"testing"
)

func TestForbiddenCalls(t *testing.T) {
	// Arrange
	root := "testdata/forbidden-bad"

	// Act
	got, err := ForbiddenCalls(root)
	// Assert
	if err != nil {
		t.Fatalf("ForbiddenCalls(%q) がエラーを返しました: %v", root, err)
	}
	// 9〜22 行目が禁止する API、26〜33 行目が PrivacyStatus へ unlisted 以外を入れる形。
	var wantPrefixes []string
	for _, line := range []int{9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 26, 27, 28, 30, 31, 33} {
		wantPrefixes = append(wantPrefixes, fmt.Sprintf("a.go:%d: ", line))
	}
	if len(got) != len(wantPrefixes) {
		t.Fatalf("ForbiddenCalls(%q) は %d 件のはずが %d 件でした（_test.go は数えない）: %q", root, len(wantPrefixes), len(got), got)
	}
	for i, prefix := range wantPrefixes {
		if !strings.HasPrefix(got[i], prefix) {
			t.Errorf("got[%d] = %q, 期待する書き出しは %q", i, got[i], prefix)
		}
	}
}

func TestForbiddenCalls_違反が無ければ何も返さない(t *testing.T) {
	// Arrange
	// trash、node_modules、. で始まるディレクトリには違反を置いてあるが、見ない。
	root := "testdata/forbidden-ok"

	// Act
	got, err := ForbiddenCalls(root)
	// Assert
	if err != nil {
		t.Fatalf("ForbiddenCalls(%q) がエラーを返しました: %v", root, err)
	}
	if len(got) != 0 {
		t.Errorf("ForbiddenCalls(%q) は何も返さないはずが %q を返しました", root, got)
	}
}
