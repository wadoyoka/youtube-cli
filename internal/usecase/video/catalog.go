// Package video は、動画に関する操作の手順を持つ。
package video

import (
	"context"
	"slices"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// Catalog は、このツールで上げた動画の一覧を引く。
type Catalog struct {
	Remote usecase.Remote
}

// Videos は、このツールで上げた動画（目印のタグがあるもの）を、PublishedAt の新しい順に返す。
// YouTube の一覧 API は上げた直後の動画を返すのが遅れるため、アップロード済みの再生リストと、この PC の ledger の ID を合わせて引く。
// ledger にあっても YouTube で見つからない動画は含めない。
func (c Catalog) Videos(ctx context.Context, s *usecase.Session) ([]domain.Video, error) {
	ids, err := s.YT.ListPlaylistVideoIDs(ctx, s.Channel.UploadsPlaylistID)
	if err != nil {
		return nil, err
	}
	recorded, err := c.ledgerIDs(ctx)
	if err != nil {
		return nil, err
	}
	ids = slices.Compact(slices.Sorted(slices.Values(append(ids, recorded...))))
	all, err := s.YT.GetVideos(ctx, ids)
	if err != nil {
		return nil, err
	}
	mine := all[:0:0]
	for _, v := range all {
		if dv.HasMarker(v.Tags) {
			mine = append(mine, v)
		}
	}
	slices.SortStableFunc(mine, func(a, b domain.Video) int { return b.PublishedAt.Compare(a.PublishedAt) })
	return mine, nil
}

// ByTitle は、Videos のうち、タイトルが title と同じものを返す。
func (c Catalog) ByTitle(ctx context.Context, s *usecase.Session, title string) ([]domain.Video, error) {
	all, err := c.Videos(ctx, s)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(v domain.Video) bool { return v.Title != title }), nil
}

// ledgerIDs は、ledger に記録した動画の ID を、ロックして読んで返す。
func (c Catalog) ledgerIDs(ctx context.Context) ([]string, error) {
	unlock, err := c.Remote.Locker.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	vs, err := c.Remote.Ledger.Videos()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(vs))
	for _, v := range vs {
		ids = append(ids, v.ID)
	}
	return ids, nil
}
