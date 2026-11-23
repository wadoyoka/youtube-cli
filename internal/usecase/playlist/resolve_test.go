package playlist_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
)

func TestCatalogResolve(t *testing.T) {
	ctx := context.Background()

	t.Run("ID で引ける", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Playlists: []domain.Playlist{cliPlaylist("PL1", "A"), cliPlaylist("PL2", "B")}}
		c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		got, err := c.Resolve(ctx, s, "PL2")

		// Assert
		if err != nil || got.ID != "PL2" || got.Title != "B" {
			t.Errorf("got=%+v err=%v", got, err)
		}
	})

	t.Run("名前で引ける", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Playlists: []domain.Playlist{cliPlaylist("PL1", "Go 入門"), cliPlaylist("PL2", "B")}}
		c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		got, err := c.Resolve(ctx, s, "Go 入門")

		// Assert
		if err != nil || got.ID != "PL1" {
			t.Errorf("got=%+v err=%v", got, err)
		}
	})

	t.Run("ledger にだけある再生リストも名前で引ける", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{LatePlaylists: []domain.Playlist{cliPlaylist("PLnew", "新")}}
		ledger := &usecasetest.Ledger{PlaylistList: []usecase.LedgerPlaylist{{ID: "PLnew", Title: "新"}}}
		c, s := newCatalog(yt, ledger, &usecasetest.Locker{})

		// Act
		got, err := c.Resolve(ctx, s, "新")

		// Assert
		if err != nil || got.ID != "PLnew" {
			t.Errorf("got=%+v err=%v", got, err)
		}
	})

	t.Run("ID が一致すれば、その ID と同じ名前のほかの再生リストがあっても ID のほうを返す", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Playlists: []domain.Playlist{cliPlaylist("PL1", "PL2"), cliPlaylist("PL2", "B")}}
		c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		got, err := c.Resolve(ctx, s, "PL2")

		// Assert
		if err != nil || got.ID != "PL2" {
			t.Errorf("got=%+v err=%v", got, err)
		}
	})

	t.Run("0件は KindNotFound", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Playlists: []domain.Playlist{cliPlaylist("PL1", "A")}}
		c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		_, err := c.Resolve(ctx, s, "ない")

		// Assert
		if domain.KindOf(err) != domain.KindNotFound || !strings.Contains(err.Error(), "ない") {
			t.Errorf("err=%v", err)
		}
	})

	t.Run("目印のない再生リストは、ID を指定しても KindNotFound", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Playlists: []domain.Playlist{{ID: "PLx", Title: "手で作った", Description: "説明", PrivacyStatus: dv.PrivacyUnlisted}}}
		c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		_, err := c.Resolve(ctx, s, "PLx")

		// Assert
		if domain.KindOf(err) != domain.KindNotFound {
			t.Errorf("err=%v", err)
		}
	})

	t.Run("同じ名前の CLI 再生リストが2つあれば KindAmbiguous で、候補の ID と URL を Details に入れる", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Playlists: []domain.Playlist{cliPlaylist("PL1", "同じ"), cliPlaylist("PL2", "同じ")}}
		c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		_, err := c.Resolve(ctx, s, "同じ")

		// Assert
		if domain.KindOf(err) != domain.KindAmbiguous {
			t.Fatalf("err=%v", err)
		}
		de, _ := errors.AsType[*domain.Error](err)
		details := strings.Join(de.Details, "\n")
		for _, want := range []string{"PL1", "PL2", "https://www.youtube.com/playlist?list=PL1", "https://www.youtube.com/playlist?list=PL2"} {
			if !strings.Contains(details, want) {
				t.Errorf("details に %s がない: %v", want, de.Details)
			}
		}
	})
}

func TestCatalogResolveForAdd(t *testing.T) {
	ctx := context.Background()

	t.Run("限定公開の CLI 再生リストを返す", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Playlists: []domain.Playlist{cliPlaylist("PL1", "A")}}
		c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		got, err := c.ResolveForAdd(ctx, s, "A")

		// Assert
		if err != nil || got.ID != "PL1" {
			t.Errorf("got=%+v err=%v", got, err)
		}
	})

	for _, privacy := range []string{"public", "private"} {
		t.Run("Studio で "+privacy+" に変えられていたら KindDenied", func(t *testing.T) {
			// Arrange
			p := cliPlaylist("PL1", "A")
			p.PrivacyStatus = privacy
			yt := &usecasetest.YouTube{Playlists: []domain.Playlist{p}}
			c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

			// Act
			_, err := c.ResolveForAdd(ctx, s, "A")

			// Assert
			if domain.KindOf(err) != domain.KindDenied || !strings.Contains(err.Error(), privacy) {
				t.Errorf("err=%v", err)
			}
		})
	}

	t.Run("見つからなければ Resolve と同じ KindNotFound", func(t *testing.T) {
		// Arrange
		c, s := newCatalog(&usecasetest.YouTube{}, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		_, err := c.ResolveForAdd(ctx, s, "ない")

		// Assert
		if domain.KindOf(err) != domain.KindNotFound {
			t.Errorf("err=%v", err)
		}
	})
}
