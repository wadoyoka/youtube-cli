// Package playlist は、このツールで作る限定公開の再生リストに関する操作の手順を持つ。
package playlist

import (
	"context"
	"slices"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// Catalog は、このツールで作った再生リストの一覧を引く。
type Catalog struct {
	Remote usecase.Remote
}

// Playlists は、このツールで作った再生リスト（説明の最後に目印があるもの）を返す。
// YouTube の一覧 API は作った直後の再生リストを返すのが遅れるため、この PC の ledger にあって一覧にない ID を、ID で引いて後ろに足す。
// ledger にあっても YouTube で見つからない（Studio で消された）再生リストは含めない。
func (c Catalog) Playlists(ctx context.Context, s *usecase.Session) ([]domain.Playlist, error) {
	listed, err := s.YT.ListMyPlaylists(ctx)
	if err != nil {
		return nil, err
	}
	recorded, err := c.ledgerIDs(ctx)
	if err != nil {
		return nil, err
	}
	missing := slices.DeleteFunc(slices.Compact(slices.Sorted(slices.Values(recorded))), func(id string) bool {
		return slices.ContainsFunc(listed, func(p domain.Playlist) bool { return p.ID == id })
	})
	late, err := s.YT.GetPlaylists(ctx, missing)
	if err != nil {
		return nil, err
	}
	// Concat で新しいスライスを作ってから除く（YouTube から受け取ったスライスを書き換えない）。
	return slices.DeleteFunc(slices.Concat(listed, late), func(p domain.Playlist) bool { return !dv.HasPlaylistMarker(p.Description) }), nil
}

// ledgerIDs は、ledger に記録した再生リストの ID を、ロックして読んで返す。
func (c Catalog) ledgerIDs(ctx context.Context) ([]string, error) {
	unlock, err := c.Remote.Locker.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	ps, err := c.Remote.Ledger.Playlists()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, p.ID)
	}
	return ids, nil
}
