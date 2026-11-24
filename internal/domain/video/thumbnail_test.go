package video_test

import (
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/domain/video"
)

func TestValidateThumbnail(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		size     int64
		wantMime string
		wantErr  bool
	}{
		{"jpgはimage/jpeg", "a.jpg", 10, "image/jpeg", false},
		{"jpegはimage/jpeg", "a.JPEG", 10, "image/jpeg", false},
		{"pngはimage/png", "a.png", 10, "image/png", false},
		{"gifは使い方の誤り", "a.gif", 10, "", true},
		{"2MiBちょうどは通る", "a.png", 2097152, "image/png", false},
		{"2MiBを1バイト超えると使い方の誤り", "a.png", 2097153, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			// Act
			mime, err := video.ValidateThumbnail(tt.file, tt.size)
			// Assert
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && domain.KindOf(err) != domain.KindUsage {
				t.Errorf("kind = %v, want usage", domain.KindOf(err))
			}
			if mime != tt.wantMime {
				t.Errorf("mime = %q, want %q", mime, tt.wantMime)
			}
		})
	}
}
