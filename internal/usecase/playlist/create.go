package playlist

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// Create は、限定公開の再生リストを作る。
type Create struct {
	Remote usecase.Remote
}

// CreateInput は、Create の入力。
type CreateInput struct {
	Title       string
	Description string // 目印の行は足す
}

// CreateResult は、Create の結果。
type CreateResult struct {
	Playlist domain.Playlist
	Warnings []string
}

// Run は、同じ名前の、このツールで作った再生リストがないことを確かめてから、説明の最後に目印を足して、限定公開の再生リストを作る。
//  1. 名前が空なら KindUsage
//  2. 同じ名前のものがあれば KindConflict（ledger にだけあるものも含める）
//  3. 作ったら、すぐに ledger に記録する。記録できなくても、作った再生リストは警告を付けて返す（やり直しで二重に作らないため）
//
// ledger のロックは、記録するときだけ取る（Catalog が自分でロックを取るので、その間は持たない）。
func (u Create) Run(ctx context.Context, in CreateInput) (CreateResult, error) {
	if strings.TrimSpace(in.Title) == "" {
		return CreateResult{}, domain.Errorf(domain.KindUsage, "再生リストの名前を指定してください")
	}
	s, err := u.Remote.Connect(ctx)
	if err != nil {
		return CreateResult{}, err
	}
	all, err := Catalog(u).Playlists(ctx, s)
	if err != nil {
		return CreateResult{}, err
	}
	if same := slices.DeleteFunc(all, func(p domain.Playlist) bool { return p.Title != in.Title }); len(same) > 0 {
		details := make([]string, 0, len(same))
		for _, p := range same {
			details = append(details, fmt.Sprintf("%s %s", p.ID, p.URL()))
		}
		return CreateResult{}, domain.Errorf(domain.KindConflict, "同じ名前の、このツールで作った再生リストがすでにあります: %s", in.Title).WithDetails(details...)
	}
	p, err := s.YT.CreatePlaylist(ctx, in.Title, dv.AddPlaylistMarker(in.Description))
	if err != nil {
		return CreateResult{}, err
	}
	res := CreateResult{Playlist: p}
	if err := u.record(ctx, usecase.LedgerPlaylist{ID: p.ID, Title: in.Title}); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("作った再生リストを手元の記録に足せませんでした（しばらくの間、playlist ls や名前での指定に出ないことがあります）: %v", err))
	}
	return res, nil
}

// record は、作った再生リストを ledger に記録する。
func (u Create) record(ctx context.Context, p usecase.LedgerPlaylist) error {
	unlock, err := u.Remote.Locker.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return u.Remote.Ledger.AddPlaylist(p)
}
