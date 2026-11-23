package video_test

import (
	"context"
	"errors"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
	"github.com/wadoyoka/youtube-cli/internal/usecase/video"
)

func newThumbnail(yt *usecasetest.YouTube, files usecasetest.Files) (video.Thumbnail, *usecasetest.Locker) {
	locker := &usecasetest.Locker{}
	r := usecase.Remote{Open: usecasetest.Opener(yt), State: &usecasetest.State{Saved: testChannel, Found: true}, Ledger: &usecasetest.Ledger{}, Locker: locker}
	return video.Thumbnail{Remote: r, Open: files.Open}, locker
}

func TestThumbnail(t *testing.T) {
	ctx := context.Background()

	t.Run("目印のある動画に画像を設定し、動画 ID と URL を返す", func(t *testing.T) {
		// Arrange
		yt := listed(cliVideo("v1", "intro", "processed"))
		th, locker := newThumbnail(yt, clip)

		// Act
		res, err := th.Run(ctx, video.ThumbnailInput{VideoID: "v1", Path: "/v/thumb.png"})

		// Assert
		if err != nil || res.VideoID != "v1" || res.URL != "https://youtu.be/v1" {
			t.Fatalf("res=%+v err=%v", res, err)
		}
		if len(yt.Thumbnails) != 1 || yt.Thumbnails[0].VideoID != "v1" || yt.Thumbnails[0].Image != "PNG" || yt.Thumbnails[0].MimeType != "image/png" {
			t.Errorf("thumbnails=%+v", yt.Thumbnails)
		}
		if locker.Locks != 1 {
			t.Errorf("目印の確かめでだけロックするはず: locks=%d", locker.Locks)
		}
	})

	t.Run("目印のない動画は KindNotFound で、API を呼ばない", func(t *testing.T) {
		// Arrange
		other := domain.Video{ID: "x1", Title: "他人の動画", Tags: []string{"go"}}
		yt := listed(cliVideo("v1", "intro", "processed"))
		yt.Videos = append(yt.Videos, other)
		th, _ := newThumbnail(yt, clip)

		// Act
		_, err := th.Run(ctx, video.ThumbnailInput{VideoID: "x1", Path: "/v/thumb.png"})

		// Assert
		if domain.KindOf(err) != domain.KindNotFound || count(yt.Calls, "SetThumbnail") != 0 {
			t.Errorf("err=%v calls=%v", err, yt.Calls)
		}
	})

	t.Run("画像が 2 MiB を超えたら KindUsage で、API を呼ばない", func(t *testing.T) {
		// Arrange
		yt := listed(cliVideo("v1", "intro", "processed"))
		big := usecasetest.Files{"/v/big.png": string(make([]byte, 2*1024*1024+1))}
		th, _ := newThumbnail(yt, big)

		// Act
		_, err := th.Run(ctx, video.ThumbnailInput{VideoID: "v1", Path: "/v/big.png"})

		// Assert
		if domain.KindOf(err) != domain.KindUsage || len(yt.Calls) != 0 {
			t.Errorf("err=%v calls=%v", err, yt.Calls)
		}
	})

	t.Run("画像がなければ KindNotFound", func(t *testing.T) {
		// Arrange
		yt := listed(cliVideo("v1", "intro", "processed"))
		th, _ := newThumbnail(yt, clip)

		// Act
		_, err := th.Run(ctx, video.ThumbnailInput{VideoID: "v1", Path: "/v/none.png"})

		// Assert
		if domain.KindOf(err) != domain.KindNotFound || len(yt.Calls) != 0 {
			t.Errorf("err=%v calls=%v", err, yt.Calls)
		}
	})

	t.Run("YouTube が断ったエラーは、そのまま返す", func(t *testing.T) {
		// Arrange
		yt := listed(cliVideo("v1", "intro", "processed"))
		yt.ThumbnailErr = domain.Errorf(domain.KindUnexpected, "電話番号の確認が必要です").Wrap(errors.New("403"))
		th, _ := newThumbnail(yt, clip)

		// Act
		_, err := th.Run(ctx, video.ThumbnailInput{VideoID: "v1", Path: "/v/thumb.png"})

		// Assert
		if domain.KindOf(err) != domain.KindUnexpected || err.Error() == "" {
			t.Errorf("err=%v", err)
		}
	})
}
