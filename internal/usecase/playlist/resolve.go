package playlist

import (
	"context"
	"fmt"
	"slices"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// Resolve は、ref（ID か名前）を、このツールで作った再生リストに解決する。
// ID が一致するものがあればそれを、なければ名前が一致するものを返す。
// 見つからなければ KindNotFound、同じ名前のものが2つ以上あれば、候補の ID と URL を Details に入れて KindAmbiguous。
// ledger のロックは Playlists が自分で取る（呼ぶ側はロックを持たないこと）。
func (c Catalog) Resolve(ctx context.Context, s *usecase.Session, ref string) (domain.Playlist, error) {
	all, err := c.Playlists(ctx, s)
	if err != nil {
		return domain.Playlist{}, err
	}
	if i := slices.IndexFunc(all, func(p domain.Playlist) bool { return p.ID == ref }); i >= 0 {
		return all[i], nil
	}
	named := slices.DeleteFunc(all, func(p domain.Playlist) bool { return p.Title != ref })
	switch len(named) {
	case 0:
		return domain.Playlist{}, domain.Errorf(domain.KindNotFound, "このツールで作った再生リストに見つかりません: %s", ref)
	case 1:
		return named[0], nil
	}
	details := make([]string, 0, len(named))
	for _, p := range named {
		details = append(details, fmt.Sprintf("%s %s", p.ID, p.URL()))
	}
	return domain.Playlist{}, domain.Errorf(domain.KindAmbiguous,
		"同じ名前の、このツールで作った再生リストが %d つあります。ID で指定してください: %s", len(named), ref).WithDetails(details...)
}

// ResolveForAdd は、ref を、動画を入れてよい再生リスト（このツールで作った限定公開のもの）に解決する。
// Studio で公開や非公開に変えられていたら、限定公開の動画が見つかるようになるのを防ぐため、KindDenied で入れずに止める。
func (c Catalog) ResolveForAdd(ctx context.Context, s *usecase.Session, ref string) (domain.Playlist, error) {
	p, err := c.Resolve(ctx, s, ref)
	if err != nil {
		return domain.Playlist{}, err
	}
	if p.PrivacyStatus != dv.PrivacyUnlisted {
		return domain.Playlist{}, domain.Errorf(domain.KindDenied,
			"再生リスト %s は限定公開（%s）ではなく %q です。動画を入れられるのは、このツールで作った限定公開の再生リストだけです", p.Title, dv.PrivacyUnlisted, p.PrivacyStatus).
			WithDetails(p.URL())
	}
	return p, nil
}
