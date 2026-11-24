package playlist_test

import (
	"context"
	"slices"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/playlist"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
)

var testChannel = domain.Channel{ID: "UC1", Title: "ch", UploadsPlaylistID: "UU1"}

// cliPlaylist は、このツールで作った（説明の最後に目印がある）限定公開の再生リスト。
func cliPlaylist(id, title string) domain.Playlist {
	return domain.Playlist{ID: id, Title: title, Description: dv.AddPlaylistMarker("説明"), PrivacyStatus: dv.PrivacyUnlisted}
}

// newRemote は、偽の YouTube・ledger・ロックで Remote を作る。
func newRemote(yt *usecasetest.YouTube, ledger *usecasetest.Ledger, locker *usecasetest.Locker) usecase.Remote {
	yt.Channel = testChannel
	return usecase.Remote{Open: usecasetest.Opener(yt), State: &usecasetest.State{Saved: testChannel, Found: true}, Ledger: ledger, Locker: locker}
}

func newCatalog(yt *usecasetest.YouTube, ledger *usecasetest.Ledger, locker *usecasetest.Locker) (playlist.Catalog, *usecase.Session) {
	return playlist.Catalog{Remote: newRemote(yt, ledger, locker)}, &usecase.Session{YT: yt, Channel: testChannel}
}

func playlistIDs(ps []domain.Playlist) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

func count(calls []string, name string) int {
	n := 0
	for _, c := range calls {
		if c == name {
			n++
		}
	}
	return n
}

func TestCatalogPlaylists(t *testing.T) {
	ctx := context.Background()

	t.Run("目印のない再生リストは出ない", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Playlists: []domain.Playlist{
			cliPlaylist("PL1", "A"),
			{ID: "PL2", Title: "手で作った", Description: "説明", PrivacyStatus: dv.PrivacyUnlisted},
			{ID: "PL3", Title: "途中に目印", Description: dv.PlaylistMarker + "\n続き", PrivacyStatus: dv.PrivacyUnlisted},
		}}
		c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		got, err := c.Playlists(ctx, s)

		// Assert
		if err != nil || !slices.Equal(playlistIDs(got), []string{"PL1"}) {
			t.Errorf("got=%v err=%v", playlistIDs(got), err)
		}
	})

	t.Run("API の一覧になく ledger にだけある再生リストが、ロックして読んだ記録から出る", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{
			Playlists:     []domain.Playlist{cliPlaylist("PL1", "A")},
			LatePlaylists: []domain.Playlist{cliPlaylist("PLnew", "新")},
		}
		ledger := &usecasetest.Ledger{PlaylistList: []usecase.LedgerPlaylist{{ID: "PLnew", Title: "新"}}}
		locker := &usecasetest.Locker{}
		c, s := newCatalog(yt, ledger, locker)

		// Act
		got, err := c.Playlists(ctx, s)

		// Assert
		if err != nil || !slices.Equal(playlistIDs(got), []string{"PL1", "PLnew"}) {
			t.Errorf("got=%v err=%v", playlistIDs(got), err)
		}
		if locker.Locks != 1 {
			t.Errorf("ledger を読むときにロックする: Locks=%d", locker.Locks)
		}
	})

	t.Run("ledger にあっても API で見つからない（Studio で消された）再生リストは出ない", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{}
		ledger := &usecasetest.Ledger{PlaylistList: []usecase.LedgerPlaylist{{ID: "gone", Title: "消えた"}}}
		c, s := newCatalog(yt, ledger, &usecasetest.Locker{})

		// Act
		got, err := c.Playlists(ctx, s)

		// Assert
		if err != nil || len(got) != 0 {
			t.Errorf("got=%v err=%v", playlistIDs(got), err)
		}
	})

	t.Run("一覧と ledger の両方にある再生リストは1回だけ出て、ID で引き直さない", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Playlists: []domain.Playlist{cliPlaylist("PL1", "A")}}
		ledger := &usecasetest.Ledger{PlaylistList: []usecase.LedgerPlaylist{{ID: "PL1", Title: "A"}}}
		c, s := newCatalog(yt, ledger, &usecasetest.Locker{})

		// Act
		got, err := c.Playlists(ctx, s)

		// Assert
		if err != nil || !slices.Equal(playlistIDs(got), []string{"PL1"}) {
			t.Errorf("got=%v err=%v", playlistIDs(got), err)
		}
		if len(yt.GotPlaylistIDs) != 0 {
			t.Errorf("一覧にあるものは引き直さない: %v", yt.GotPlaylistIDs)
		}
	})

	t.Run("一覧の取得に失敗したらそのエラーを返す", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{PlaylistsErr: domain.Errorf(domain.KindQuota, "quota")}
		c, s := newCatalog(yt, &usecasetest.Ledger{}, &usecasetest.Locker{})

		// Act
		_, err := c.Playlists(ctx, s)

		// Assert
		if domain.KindOf(err) != domain.KindQuota {
			t.Errorf("err=%v", err)
		}
	})
}
