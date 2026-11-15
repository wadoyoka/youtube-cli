package gcp

import (
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

func TestValidateProjectID(t *testing.T) {
	cases := []struct {
		name string
		id   string
		ok   bool
	}{
		{"通る: 英数とハイフン", "youtube-cli-a1b2c3", true},
		{"通る: 6文字", "abcdef", true},
		{"通る: 30文字", strings.Repeat("a", 30), true},
		{"通らない: 短い", "abcde", false},
		{"通らない: 31文字", strings.Repeat("a", 31), false},
		{"通らない: 大文字", "Youtube-cli", false},
		{"通らない: 数字で始まる", "1abcdef", false},
		{"通らない: アンダースコア", "abc_def", false},
		{"通らない: ハイフンで終わる", "abcdef-", false},
		{"通らない: 空", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Arrange
			// Act
			err := ValidateProjectID(c.id)
			// Assert
			if (err == nil) != c.ok {
				t.Fatalf("ValidateProjectID(%q) = %v, ok want %v", c.id, err, c.ok)
			}
			if err != nil && domain.KindOf(err) != domain.KindUsage {
				t.Errorf("kind = %v, want KindUsage", domain.KindOf(err))
			}
		})
	}
}

func TestAuthConsoleURL(t *testing.T) {
	t.Run("画面とプロジェクトから URL を組み立てる", func(t *testing.T) {
		// Arrange
		// Act
		got := AuthConsoleURL("clients", "p-123456")
		// Assert
		want := "https://console.cloud.google.com/auth/clients?project=p-123456"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
