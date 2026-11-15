package video_test

import (
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/domain/video"
)

func TestValidateMeta(t *testing.T) {
	tests := []struct {
		name    string
		meta    video.Meta
		wantErr bool
	}{
		{"空のタイトルは使い方の誤り", video.Meta{Title: ""}, true},
		{"100文字ちょうどのタイトルは通る", video.Meta{Title: strings.Repeat("あ", 100)}, false},
		{"101文字のタイトルは使い方の誤り", video.Meta{Title: strings.Repeat("あ", 101)}, true},
		{"小なり記号を含むタイトルは使い方の誤り", video.Meta{Title: "a<b"}, true},
		{"大なり記号を含むタイトルは使い方の誤り", video.Meta{Title: "a>b"}, true},
		{"5000バイトの説明は通る", video.Meta{Title: "t", Description: strings.Repeat("a", 5000)}, false},
		{"5001バイトの説明は使い方の誤り", video.Meta{Title: "t", Description: strings.Repeat("a", 5001)}, true},
		{"タグの合計が500ちょうどは通る", video.Meta{Title: "t", Tags: []string{strings.Repeat("a", 250), strings.Repeat("b", 249)}}, false},
		{"タグの合計が500を超えると使い方の誤り", video.Meta{Title: "t", Tags: []string{strings.Repeat("a", 250), strings.Repeat("b", 250)}}, true},
		{"空白を含むタグは引用符の2文字を足して数える", video.Meta{Title: "t", Tags: []string{"a b" + strings.Repeat("c", 496)}}, true},
		{"空白を含むタグが引用符込みで500なら通る", video.Meta{Title: "t", Tags: []string{"a b" + strings.Repeat("c", 495)}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			// Act
			err := video.ValidateMeta(tt.meta)
			// Assert
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && domain.KindOf(err) != domain.KindUsage {
				t.Errorf("kind = %v, want usage", domain.KindOf(err))
			}
		})
	}
}

func TestTitleFromPath(t *testing.T) {
	t.Run("拡張子とディレクトリを除く", func(t *testing.T) {
		// Arrange
		path := "./a/intro.mp4"
		// Act
		got := video.TitleFromPath(path)
		// Assert
		if got != "intro" {
			t.Errorf("got %q", got)
		}
	})
}

func TestWithMarker(t *testing.T) {
	t.Run("目印が1つだけ入る", func(t *testing.T) {
		// Arrange
		tags := []string{"go"}
		// Act
		got := video.WithMarker(tags)
		// Assert
		if len(got) != 2 || got[0] != "go" || got[1] != video.MarkerTag {
			t.Errorf("got %v", got)
		}
		if !video.HasMarker(got) {
			t.Error("HasMarker = false")
		}
	})
	t.Run("すでにあっても増えない", func(t *testing.T) {
		// Arrange
		tags := []string{"youtube-cli", "go"}
		// Act
		got := video.WithMarker(tags)
		// Assert
		if len(got) != 2 {
			t.Errorf("got %v", got)
		}
	})
	t.Run("目印がなければHasMarkerは偽", func(t *testing.T) {
		// Arrange
		// Act
		got := video.HasMarker([]string{"go"})
		// Assert
		if got {
			t.Error("HasMarker = true")
		}
	})
}
