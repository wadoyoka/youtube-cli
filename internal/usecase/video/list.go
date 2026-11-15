package video

import (
	"context"
	"slices"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/playlist"
)

// List は、このツールで上げた動画を一覧にする。
type List struct {
	Remote usecase.Remote
}

// ListInput は、List の入力。
type ListInput struct {
	PlaylistRef string // 空でなければ、このツールで作ったこの再生リスト（ID か名前）に入っている動画だけにする
}

// Run は、このツールで上げた動画を、新しい順に返す。
// PlaylistRef の再生リストは playlist.Catalog.Resolve で解決する（見つからなければ KindNotFound、あいまいなら KindAmbiguous）。
// ledger のロックは、Resolve と Catalog がそれぞれ自分で取る（その間は持たない）。
func (u List) Run(ctx context.Context, in ListInput) ([]domain.Video, error) {
	s, err := u.Remote.Connect(ctx)
	if err != nil {
		return nil, err
	}
	var inPlaylist []string
	if in.PlaylistRef != "" {
		p, err := playlist.Catalog{Remote: u.Remote}.Resolve(ctx, s, in.PlaylistRef)
		if err != nil {
			return nil, err
		}
		if inPlaylist, err = s.YT.ListPlaylistVideoIDs(ctx, p.ID); err != nil {
			return nil, err
		}
	}
	mine, err := Catalog(u).Videos(ctx, s)
	if err != nil || in.PlaylistRef == "" {
		return mine, err
	}
	return slices.DeleteFunc(mine, func(v domain.Video) bool { return !slices.Contains(inPlaylist, v.ID) }), nil
}
