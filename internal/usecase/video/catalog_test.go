package video_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
	"github.com/wadoyoka/youtube-cli/internal/usecase/video"
)

var testChannel = domain.Channel{ID: "UC1", Title: "ch", UploadsPlaylistID: "UU1"}

func at(day int) time.Time { return time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC) }

func ours(id, title string, day int) domain.Video {
	return domain.Video{ID: id, Title: title, Tags: []string{"x", dv.MarkerTag}, PublishedAt: at(day)}
}

func newCatalog(yt *usecasetest.YouTube, ledger *usecasetest.Ledger, locker *usecasetest.Locker) (video.Catalog, *usecase.Session) {
	r := usecase.Remote{Open: usecasetest.Opener(yt), State: &usecasetest.State{Saved: testChannel, Found: true}, Ledger: ledger, Locker: locker}
	return video.Catalog{Remote: r}, &usecase.Session{YT: yt, Channel: testChannel}
}

func ids(vs []domain.Video) []string {
	var out []string
	for _, v := range vs {
		out = append(out, v.ID)
	}
	return out
}

func TestCatalogVideos(t *testing.T) {
	t.Run("目印のない動画は出ない", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{
			PlaylistVideoIDs: map[string][]string{"UU1": {"a", "b"}},
			Videos:           []domain.Video{ours("a", "A", 1), {ID: "b", Title: "B", Tags: []string{"other"}}},
		}
		c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		got, err := c.Videos(context.Background(), s)

		// Assert
		if err != nil || !slices.Equal(ids(got), []string{"a"}) {
			t.Errorf("got=%v err=%v", ids(got), err)
		}
	})

	t.Run("API の一覧になく ledger にだけある動画が出る", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Videos: []domain.Video{ours("n", "新", 2)}}
		ledger := &usecasetest.Ledger{VideoList: []usecase.LedgerVideo{{ID: "n"}}}
		locker := &usecasetest.Locker{}
		c, s := newCatalog(yt, ledger, locker)

		// Act
		got, err := c.Videos(context.Background(), s)

		// Assert
		if err != nil || !slices.Equal(ids(got), []string{"n"}) {
			t.Errorf("got=%v err=%v", ids(got), err)
		}
		if locker.Locks != 1 {
			t.Errorf("ledger を読むときにロックする: Locks=%d", locker.Locks)
		}
	})

	t.Run("ledger にあっても API で見つからない動画は出ない", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{}
		ledger := &usecasetest.Ledger{VideoList: []usecase.LedgerVideo{{ID: "gone"}}}
		c, s := newCatalog(yt, ledger, &usecasetest.Locker{})

		// Act
		got, err := c.Videos(context.Background(), s)

		// Assert
		if err != nil || len(got) != 0 {
			t.Errorf("got=%v err=%v", ids(got), err)
		}
	})

	t.Run("同じ動画は1回だけ出る", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{
			PlaylistVideoIDs: map[string][]string{"UU1": {"a"}},
			Videos:           []domain.Video{ours("a", "A", 1)},
		}
		ledger := &usecasetest.Ledger{VideoList: []usecase.LedgerVideo{{ID: "a"}}}
		c, s := newCatalog(yt, ledger, &usecasetest.Locker{})

		// Act
		got, err := c.Videos(context.Background(), s)

		// Assert
		if err != nil || !slices.Equal(ids(got), []string{"a"}) {
			t.Errorf("got=%v err=%v", ids(got), err)
		}
	})

	t.Run("PublishedAt の新しい順に並ぶ", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{
			PlaylistVideoIDs: map[string][]string{"UU1": {"old", "mid"}},
			Videos:           []domain.Video{ours("old", "o", 1), ours("mid", "m", 2), ours("new", "n", 3)},
		}
		ledger := &usecasetest.Ledger{VideoList: []usecase.LedgerVideo{{ID: "new"}}}
		c, s := newCatalog(yt, ledger, &usecasetest.Locker{})

		// Act
		got, err := c.Videos(context.Background(), s)

		// Assert
		if err != nil || !slices.Equal(ids(got), []string{"new", "mid", "old"}) {
			t.Errorf("got=%v err=%v", ids(got), err)
		}
	})

	t.Run("一覧の取得に失敗したらそのエラーを返す", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{ListErr: domain.Errorf(domain.KindQuota, "quota")}
		c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		_, err := c.Videos(context.Background(), s)

		// Assert
		if domain.KindOf(err) != domain.KindQuota {
			t.Errorf("err=%v", err)
		}
	})
}

func TestCatalogByTitle(t *testing.T) {
	t.Run("同じタイトルの動画だけを返す", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{
			PlaylistVideoIDs: map[string][]string{"UU1": {"a", "b", "c"}},
			Videos:           []domain.Video{ours("a", "同じ", 1), ours("b", "別", 2), ours("c", "同じ", 3)},
		}
		c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		got, err := c.ByTitle(context.Background(), s, "同じ")

		// Assert
		if err != nil || !slices.Equal(ids(got), []string{"c", "a"}) {
			t.Errorf("got=%v err=%v", ids(got), err)
		}
	})
}

func TestList(t *testing.T) {
	t.Run("接続して目印のある動画を返す", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{
			Channel:          testChannel,
			PlaylistVideoIDs: map[string][]string{"UU1": {"a"}},
			Videos:           []domain.Video{ours("a", "A", 1)},
		}
		r := usecase.Remote{Open: usecasetest.Opener(yt), State: &usecasetest.State{Saved: testChannel, Found: true}, Ledger: &usecasetest.Ledger{}, Locker: &usecasetest.Locker{}}

		// Act
		got, err := video.List{Remote: r}.Run(context.Background(), video.ListInput{})

		// Assert
		if err != nil || !slices.Equal(ids(got), []string{"a"}) {
			t.Errorf("got=%v err=%v", ids(got), err)
		}
	})

	t.Run("PlaylistRef を指定すると、その CLI 再生リストに入っている CLI 動画だけを返す", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{
			Channel: testChannel,
			PlaylistVideoIDs: map[string][]string{
				"UU1": {"a", "b", "c"},
				"PL1": {"c", "a", "x"},
			},
			Videos: []domain.Video{
				ours("a", "A", 1), ours("b", "B", 2), ours("c", "C", 3),
				{ID: "x", Title: "他人", Tags: []string{"go"}},
			},
			Playlists: []domain.Playlist{{ID: "PL1", Title: "Go 入門", Description: dv.AddPlaylistMarker(""), PrivacyStatus: "public"}},
		}
		r := usecase.Remote{Open: usecasetest.Opener(yt), State: &usecasetest.State{Saved: testChannel, Found: true}, Ledger: &usecasetest.Ledger{}, Locker: &usecasetest.Locker{}}

		// Act
		got, err := video.List{Remote: r}.Run(context.Background(), video.ListInput{PlaylistRef: "Go 入門"})

		// Assert
		if err != nil || !slices.Equal(ids(got), []string{"c", "a"}) {
			t.Errorf("got=%v err=%v", ids(got), err)
		}
	})

	t.Run("PlaylistRef が CLI の再生リストでなければ KindNotFound", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{
			Channel:   testChannel,
			Playlists: []domain.Playlist{{ID: "PL1", Title: "手で作った", Description: "説明"}},
		}
		r := usecase.Remote{Open: usecasetest.Opener(yt), State: &usecasetest.State{Saved: testChannel, Found: true}, Ledger: &usecasetest.Ledger{}, Locker: &usecasetest.Locker{}}

		// Act
		_, err := video.List{Remote: r}.Run(context.Background(), video.ListInput{PlaylistRef: "PL1"})

		// Assert
		if domain.KindOf(err) != domain.KindNotFound {
			t.Errorf("err=%v", err)
		}
	})
}
