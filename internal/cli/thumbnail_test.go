package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
	"github.com/wadoyoka/youtube-cli/internal/usecase/video"
)

// withThumbnail は、偽の YouTube に目印付きの動画 v1 がある Thumbnail を d に入れる。
func withThumbnail(d Deps) (Deps, *usecasetest.YouTube) {
	v1 := domain.Video{ID: "v1", Title: "intro", Tags: []string{dv.MarkerTag}}
	yt := &usecasetest.YouTube{
		Channel: testChannel, Videos: []domain.Video{v1, {ID: "x1", Title: "他人"}},
		PlaylistVideoIDs: map[string][]string{testChannel.UploadsPlaylistID: {"v1", "x1"}},
	}
	d.Thumbnail = video.Thumbnail{Remote: usecase.Remote{
		Open:   usecasetest.Opener(yt),
		State:  &usecasetest.State{Saved: testChannel, Found: true},
		Locker: &usecasetest.Locker{},
		Ledger: &usecasetest.Ledger{},
	}, Open: uploadFiles.Open}
	return d, yt
}

func TestThumbnailCommand(t *testing.T) {
	ctx := context.Background()

	t.Run("画像を設定し、動画 ID と URL を出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withThumbnail(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"thumbnail", "v1", "/v/thumb.png"})

		// Assert
		if code != 0 || len(yt.Thumbnails) != 1 || !strings.Contains(out.String(), "https://youtu.be/v1") {
			t.Errorf("code=%d out=%q stderr=%q", code, out.String(), errb.String())
		}
	})

	t.Run("--json は videoId と url を出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, _ = withThumbnail(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"--json", "thumbnail", "v1", "/v/thumb.png"})

		// Assert
		var got map[string]string
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || code != 0 {
			t.Fatalf("code=%d err=%v out=%q", code, err, out.String())
		}
		if got["videoId"] != "v1" || got["url"] != "https://youtu.be/v1" {
			t.Errorf("got=%v", got)
		}
	})

	t.Run("CLI の動画でなければ 4 で終わる", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withThumbnail(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"thumbnail", "x1", "/v/thumb.png"})

		// Assert
		if code != 4 || len(yt.Thumbnails) != 0 {
			t.Errorf("code=%d stderr=%q", code, errb.String())
		}
	})

	t.Run("引数が足りなければ 2 で終わる", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, _ = withThumbnail(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"thumbnail", "v1"})

		// Assert
		if code != 2 {
			t.Errorf("code=%d", code)
		}
	})

	t.Run("thumbnail を拒否していれば 7 で止まる", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, yt := withThumbnail(d)
		initAndClear(t, d, out, errb)
		if err := os.WriteFile(filepath.Join(dir, "policy.toml"), []byte(`deny = ["thumbnail"]`), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		code := Execute(ctx, d, []string{"thumbnail", "v1", "/v/thumb.png"})

		// Assert
		if code != 7 || len(yt.Calls) != 0 {
			t.Errorf("code=%d calls=%v", code, yt.Calls)
		}
	})
}
