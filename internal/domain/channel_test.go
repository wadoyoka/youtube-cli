package domain

import "testing"

func TestChannelURL(t *testing.T) {
	t.Run("チャンネル ID から URL を作る", func(t *testing.T) {
		// Arrange
		c := Channel{ID: "UC123", Title: "t", UploadsPlaylistID: "UU123"}

		// Act
		got := c.URL()

		// Assert
		if want := "https://www.youtube.com/channel/UC123"; got != want {
			t.Errorf("URL = %q, want %q", got, want)
		}
	})
}
