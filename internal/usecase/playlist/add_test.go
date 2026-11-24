package playlist_test

import (
	"context"
	"slices"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase/playlist"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
	"github.com/wadoyoka/youtube-cli/internal/usecase/video"
)

// withVideos は、アップロード済みの再生リストに vs が並び、再生リストが pls の偽の YouTube を返す。
func withVideos(pls []domain.Playlist, vs ...domain.Video) *usecasetest.YouTube {
	yt := &usecasetest.YouTube{Playlists: pls, Videos: vs, PlaylistVideoIDs: map[string][]string{}}
	for _, v := range vs {
		yt.PlaylistVideoIDs[testChannel.UploadsPlaylistID] = append(yt.PlaylistVideoIDs[testChannel.UploadsPlaylistID], v.ID)
	}
	return yt
}

// v1 は、このツールで上げた（目印のタグがある）動画。
var v1 = domain.Video{ID: "v1", Title: "題", Tags: []string{dv.MarkerTag}, PrivacyStatus: dv.PrivacyUnlisted, UploadStatus: "processed"}

func newAdd(yt *usecasetest.YouTube) playlist.Add {
	r := newRemote(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})
	return playlist.Add{Remote: r, Videos: video.Catalog{Remote: r}}
}

func TestAdd(t *testing.T) {
	ctx := context.Background()

	t.Run("CLI の動画を限定公開の CLI 再生リストに入れる", func(t *testing.T) {
		// Arrange
		yt := withVideos([]domain.Playlist{cliPlaylist("PL1", "Go 入門")}, v1)

		// Act
		res, err := newAdd(yt).Run(ctx, playlist.AddInput{VideoID: "v1", PlaylistRef: "Go 入門"})

		// Assert
		if err != nil || !res.Added || res.VideoID != "v1" || res.Playlist.ID != "PL1" {
			t.Fatalf("res=%+v err=%v", res, err)
		}
		if !slices.Equal(yt.AddedToPlaylist, []usecasetest.AddToPlaylistCall{{PlaylistID: "PL1", VideoID: "v1"}}) {
			t.Errorf("added=%v", yt.AddedToPlaylist)
		}
	})

	for _, privacy := range []string{"public", "private"} {
		t.Run("再生リストが "+privacy+" なら KindDenied で AddToPlaylist を呼ばない", func(t *testing.T) {
			// Arrange
			p := cliPlaylist("PL1", "Go 入門")
			p.PrivacyStatus = privacy
			yt := withVideos([]domain.Playlist{p}, v1)

			// Act
			_, err := newAdd(yt).Run(ctx, playlist.AddInput{VideoID: "v1", PlaylistRef: "PL1"})

			// Assert
			if domain.KindOf(err) != domain.KindDenied || domain.KindOf(err).ExitCode() != 7 || count(yt.Calls, "AddToPlaylist") != 0 {
				t.Errorf("err=%v calls=%v", err, yt.Calls)
			}
		})
	}

	t.Run("目印のない動画は KindNotFound で AddToPlaylist を呼ばない", func(t *testing.T) {
		// Arrange
		yt := withVideos([]domain.Playlist{cliPlaylist("PL1", "Go 入門")}, domain.Video{ID: "x1", Title: "他", Tags: []string{"go"}})

		// Act
		_, err := newAdd(yt).Run(ctx, playlist.AddInput{VideoID: "x1", PlaylistRef: "PL1"})

		// Assert
		if domain.KindOf(err) != domain.KindNotFound || count(yt.Calls, "AddToPlaylist") != 0 {
			t.Errorf("err=%v calls=%v", err, yt.Calls)
		}
	})

	t.Run("CLI の再生リストが見つからなければ KindNotFound", func(t *testing.T) {
		// Arrange
		yt := withVideos(nil, v1)

		// Act
		_, err := newAdd(yt).Run(ctx, playlist.AddInput{VideoID: "v1", PlaylistRef: "ない"})

		// Assert
		if domain.KindOf(err) != domain.KindNotFound || count(yt.Calls, "AddToPlaylist") != 0 {
			t.Errorf("err=%v calls=%v", err, yt.Calls)
		}
	})

	t.Run("同じ名前の CLI 再生リストが2つあれば KindAmbiguous", func(t *testing.T) {
		// Arrange
		yt := withVideos([]domain.Playlist{cliPlaylist("PL1", "同じ"), cliPlaylist("PL2", "同じ")}, v1)

		// Act
		_, err := newAdd(yt).Run(ctx, playlist.AddInput{VideoID: "v1", PlaylistRef: "同じ"})

		// Assert
		if domain.KindOf(err) != domain.KindAmbiguous || count(yt.Calls, "AddToPlaylist") != 0 {
			t.Errorf("err=%v calls=%v", err, yt.Calls)
		}
	})

	t.Run("すでに入っていれば何もせず成功", func(t *testing.T) {
		// Arrange
		yt := withVideos([]domain.Playlist{cliPlaylist("PL1", "Go 入門")}, v1)
		yt.PlaylistVideoIDs["PL1"] = []string{"v0", "v1"}

		// Act
		res, err := newAdd(yt).Run(ctx, playlist.AddInput{VideoID: "v1", PlaylistRef: "PL1"})

		// Assert
		if err != nil || res.Added || res.Playlist.ID != "PL1" || count(yt.Calls, "AddToPlaylist") != 0 {
			t.Errorf("res=%+v err=%v calls=%v", res, err, yt.Calls)
		}
	})

	t.Run("入れるのに失敗したらそのエラーを返す", func(t *testing.T) {
		// Arrange
		yt := withVideos([]domain.Playlist{cliPlaylist("PL1", "Go 入門")}, v1)
		yt.AddToPlaylistErr = domain.Errorf(domain.KindQuota, "quota")

		// Act
		_, err := newAdd(yt).Run(ctx, playlist.AddInput{VideoID: "v1", PlaylistRef: "PL1"})

		// Assert
		if domain.KindOf(err) != domain.KindQuota {
			t.Errorf("err=%v", err)
		}
	})
}

func TestAddIfAbsent(t *testing.T) {
	ctx := context.Background()

	t.Run("入っていなければ入れて true", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{}

		// Act
		added, err := playlist.AddIfAbsent(ctx, yt, "PL1", "v1")

		// Assert
		if err != nil || !added || len(yt.AddedToPlaylist) != 1 {
			t.Errorf("added=%v err=%v calls=%v", added, err, yt.Calls)
		}
	})

	t.Run("入っていれば何もせず false", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{PlaylistVideoIDs: map[string][]string{"PL1": {"v1"}}}

		// Act
		added, err := playlist.AddIfAbsent(ctx, yt, "PL1", "v1")

		// Assert
		if err != nil || added || len(yt.AddedToPlaylist) != 0 {
			t.Errorf("added=%v err=%v calls=%v", added, err, yt.Calls)
		}
	})

	t.Run("中身を引けなければ、そのエラーを返して入れない", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{ListErr: domain.Errorf(domain.KindQuota, "quota")}

		// Act
		_, err := playlist.AddIfAbsent(ctx, yt, "PL1", "v1")

		// Assert
		if domain.KindOf(err) != domain.KindQuota || len(yt.AddedToPlaylist) != 0 {
			t.Errorf("err=%v calls=%v", err, yt.Calls)
		}
	})
}
