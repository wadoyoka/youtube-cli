package video_test

import (
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain/video"
)

func TestAddPlaylistMarker(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"説明の後ろに空行を挟んで足す", "説明", "説明\n\n[youtube-cli]"},
		{"空の説明なら目印だけ", "", "[youtube-cli]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			// Act
			got := video.AddPlaylistMarker(tt.in)
			// Assert
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasPlaylistMarker(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"最後の行が目印なら真", "説明\n\n[youtube-cli]", true},
		{"末尾の空行は無視する", "説明\n[youtube-cli]\n\n", true},
		{"目印が途中にあるだけなら偽", "[youtube-cli]\n説明", false},
		{"空なら偽", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			// Act
			got := video.HasPlaylistMarker(tt.in)
			// Assert
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnusable(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"failed", true},
		{"rejected", true},
		{"deleted", true},
		{"uploaded", false},
		{"processed", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.status+"の判定", func(t *testing.T) {
			// Arrange
			// Act
			got := video.Unusable(tt.status)
			// Assert
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
