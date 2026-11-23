package domain_test

import (
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

func TestVideoとPlaylistのURL(t *testing.T) {
	t.Run("動画のURLを返す", func(t *testing.T) {
		// Arrange
		v := domain.Video{ID: "abc"}
		// Act
		url, studio := v.URL(), v.StudioURL()
		// Assert
		if url != "https://youtu.be/abc" {
			t.Errorf("URL = %q", url)
		}
		if studio != "https://studio.youtube.com/video/abc/edit" {
			t.Errorf("StudioURL = %q", studio)
		}
	})
	t.Run("再生リストのURLを返す", func(t *testing.T) {
		// Arrange
		p := domain.Playlist{ID: "PL1"}
		// Act
		url := p.URL()
		// Assert
		if url != "https://www.youtube.com/playlist?list=PL1" {
			t.Errorf("URL = %q", url)
		}
	})
}
