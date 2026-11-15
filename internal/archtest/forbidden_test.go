package archtest

import (
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
	wantPrefixes := []string{"a.go:20: ", "a.go:21: ", "a.go:22: ", "a.go:23: ", "a.go:25: "}
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
