package playlist_test

import (
	"context"
	"slices"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase/playlist"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
)

func TestList(t *testing.T) {
	t.Run("接続して CLI の再生リストを返す", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Playlists: []domain.Playlist{cliPlaylist("PL1", "A"), {ID: "PL2", Title: "手で作った"}}}
		u := playlist.List{Remote: newRemote(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})}

		// Act
		got, err := u.Run(context.Background())

		// Assert
		if err != nil || !slices.Equal(playlistIDs(got), []string{"PL1"}) {
			t.Errorf("got=%v err=%v", playlistIDs(got), err)
		}
	})

	t.Run("保存したチャンネルと違えば KindSetup", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{}
		r := newRemote(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})
		yt.Channel = domain.Channel{ID: "UCother"}

		// Act
		_, err := playlist.List{Remote: r}.Run(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Errorf("err=%v", err)
		}
	})
}
