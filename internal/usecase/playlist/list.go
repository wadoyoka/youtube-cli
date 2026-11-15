package playlist

import (
	"context"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// List は、このツールで作った再生リストを一覧にする。
type List struct {
	Remote usecase.Remote
}

// Run は、このツールで作った再生リストを返す。
func (u List) Run(ctx context.Context) ([]domain.Playlist, error) {
	s, err := u.Remote.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return Catalog(u).Playlists(ctx, s)
}
