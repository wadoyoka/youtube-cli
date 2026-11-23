package playlist_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/playlist"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
)

func TestCreate(t *testing.T) {
	ctx := context.Background()

	t.Run("説明の最後に目印を足して作り、ledger に足す", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{CreateResult: cliPlaylist("PLnew", "Go 入門")}
		ledger := &usecasetest.Ledger{}
		locker := &usecasetest.Locker{}
		u := playlist.Create{Remote: newRemote(yt, ledger, locker)}

		// Act
		res, err := u.Run(ctx, playlist.CreateInput{Title: "Go 入門", Description: "Go を学ぶ"})

		// Assert
		if err != nil || res.Playlist.ID != "PLnew" || len(res.Warnings) != 0 {
			t.Fatalf("res=%+v err=%v", res, err)
		}
		want := []usecasetest.CreatePlaylistCall{{Title: "Go 入門", Description: "Go を学ぶ\n\n" + dv.PlaylistMarker}}
		if !slices.Equal(yt.Created, want) {
			t.Errorf("created=%v want=%v", yt.Created, want)
		}
		if !slices.Equal(ledger.PlaylistList, []usecase.LedgerPlaylist{{ID: "PLnew", Title: "Go 入門"}}) {
			t.Errorf("ledger=%v", ledger.PlaylistList)
		}
		if locker.Locks != 2 {
			t.Errorf("一覧を引くときと記録するときにロックするはず: locks=%d", locker.Locks)
		}
	})

	t.Run("説明を省くと、目印だけの説明で作る", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{CreateResult: cliPlaylist("PLnew", "A")}
		u := playlist.Create{Remote: newRemote(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})}

		// Act
		_, err := u.Run(ctx, playlist.CreateInput{Title: "A"})

		// Assert
		if err != nil || len(yt.Created) != 1 || yt.Created[0].Description != dv.PlaylistMarker {
			t.Errorf("created=%v err=%v", yt.Created, err)
		}
	})

	t.Run("同じ名前の CLI 再生リストがあれば KindConflict で、CreatePlaylist を呼ばない", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Playlists: []domain.Playlist{cliPlaylist("PL1", "Go 入門")}}
		u := playlist.Create{Remote: newRemote(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})}

		// Act
		_, err := u.Run(ctx, playlist.CreateInput{Title: "Go 入門"})

		// Assert
		if domain.KindOf(err) != domain.KindConflict || count(yt.Calls, "CreatePlaylist") != 0 {
			t.Fatalf("err=%v calls=%v", err, yt.Calls)
		}
		if de, _ := errors.AsType[*domain.Error](err); !strings.Contains(strings.Join(de.Details, "\n"), "https://www.youtube.com/playlist?list=PL1") {
			t.Errorf("details=%v", de.Details)
		}
	})

	t.Run("ledger にだけある同じ名前の CLI 再生リストでも KindConflict", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{LatePlaylists: []domain.Playlist{cliPlaylist("PLnew", "Go 入門")}}
		ledger := &usecasetest.Ledger{PlaylistList: []usecase.LedgerPlaylist{{ID: "PLnew", Title: "Go 入門"}}}
		u := playlist.Create{Remote: newRemote(yt, ledger, &usecasetest.Locker{})}

		// Act
		_, err := u.Run(ctx, playlist.CreateInput{Title: "Go 入門"})

		// Assert
		if domain.KindOf(err) != domain.KindConflict || count(yt.Calls, "CreatePlaylist") != 0 {
			t.Errorf("err=%v calls=%v", err, yt.Calls)
		}
	})

	t.Run("目印のない同じ名前の再生リストがあっても作る", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{
			Playlists:    []domain.Playlist{{ID: "PLhand", Title: "Go 入門", Description: "手で作った", PrivacyStatus: "public"}},
			CreateResult: cliPlaylist("PLnew", "Go 入門"),
		}
		u := playlist.Create{Remote: newRemote(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})}

		// Act
		res, err := u.Run(ctx, playlist.CreateInput{Title: "Go 入門"})

		// Assert
		if err != nil || res.Playlist.ID != "PLnew" {
			t.Errorf("res=%+v err=%v", res, err)
		}
	})

	t.Run("名前が空なら KindUsage で、YouTube に接続しない", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{}
		u := playlist.Create{Remote: newRemote(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})}

		// Act
		_, err := u.Run(ctx, playlist.CreateInput{Title: "  "})

		// Assert
		if domain.KindOf(err) != domain.KindUsage || len(yt.Calls) != 0 {
			t.Errorf("err=%v calls=%v", err, yt.Calls)
		}
	})

	t.Run("作るのに失敗したらそのエラーを返し、ledger に足さない", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{CreateErr: domain.Errorf(domain.KindQuota, "quota")}
		ledger := &usecasetest.Ledger{}
		u := playlist.Create{Remote: newRemote(yt, ledger, &usecasetest.Locker{})}

		// Act
		_, err := u.Run(ctx, playlist.CreateInput{Title: "A"})

		// Assert
		if domain.KindOf(err) != domain.KindQuota || len(ledger.PlaylistList) != 0 {
			t.Errorf("err=%v ledger=%v", err, ledger.PlaylistList)
		}
	})

	t.Run("ledger に書けなくても、作った再生リストを返し、警告を入れる", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{CreateResult: cliPlaylist("PLnew", "A")}
		u := playlist.Create{Remote: newRemote(yt, &usecasetest.Ledger{AddErr: errors.New("書けない")}, &usecasetest.Locker{})}

		// Act
		res, err := u.Run(ctx, playlist.CreateInput{Title: "A"})

		// Assert
		if err != nil || res.Playlist.ID != "PLnew" || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "書けない") {
			t.Errorf("res=%+v err=%v", res, err)
		}
	})
}
