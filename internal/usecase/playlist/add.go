package playlist

import (
	"context"
	"slices"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// VideoCatalog は、このツールで上げた動画の一覧。video.Catalog が満たす。
// video は upload --playlist と ls --playlist でこのパッケージを使うので、ここからは video を import せずに受け取る。
type VideoCatalog interface {
	Videos(ctx context.Context, s *usecase.Session) ([]domain.Video, error)
}

// Add は、このツールで上げた動画を、このツールで作った限定公開の再生リストに入れる。
type Add struct {
	Remote usecase.Remote
	Videos VideoCatalog
}

// AddInput は、Add の入力。
type AddInput struct {
	VideoID     string
	PlaylistRef string // 再生リストの ID か名前
}

// AddResult は、Add の結果。
type AddResult struct {
	VideoID  string
	Playlist domain.Playlist
	Added    bool // false なら、すでに入っていたので何もしなかった
}

// Run は、動画がこのツールで上げたもので、再生リストがこのツールで作った限定公開のものであることを確かめてから、動画を入れる。
// 目印のない動画は KindNotFound。再生リストは ResolveForAdd の決まり（見つからない・あいまい・限定公開でない）で止める。
// すでに入っていれば何もせず成功する。
// ledger のロックは、Videos と ResolveForAdd がそれぞれ自分で取る（その間は持たない）。
func (u Add) Run(ctx context.Context, in AddInput) (AddResult, error) {
	s, err := u.Remote.Connect(ctx)
	if err != nil {
		return AddResult{}, err
	}
	mine, err := u.Videos.Videos(ctx, s)
	if err != nil {
		return AddResult{}, err
	}
	if !slices.ContainsFunc(mine, func(v domain.Video) bool { return v.ID == in.VideoID }) {
		return AddResult{}, domain.Errorf(domain.KindNotFound, "このツールで上げた動画に見つかりません: %s", in.VideoID)
	}
	p, err := Catalog{Remote: u.Remote}.ResolveForAdd(ctx, s, in.PlaylistRef)
	if err != nil {
		return AddResult{}, err
	}
	added, err := AddIfAbsent(ctx, s.YT, p.ID, in.VideoID)
	if err != nil {
		return AddResult{}, err
	}
	return AddResult{VideoID: in.VideoID, Playlist: p, Added: added}, nil
}

// AddIfAbsent は、動画が再生リストに入っていなければ入れて true を、すでに入っていれば何もせず false を返す。
// upload --playlist も使う（--skip-existing で既存の動画を使うとき、二重に入れないため）。
func AddIfAbsent(ctx context.Context, yt usecase.YouTube, playlistID, videoID string) (bool, error) {
	ids, err := yt.ListPlaylistVideoIDs(ctx, playlistID)
	if err != nil {
		return false, err
	}
	if slices.Contains(ids, videoID) {
		return false, nil
	}
	if err := yt.AddToPlaylist(ctx, playlistID, videoID); err != nil {
		return false, err
	}
	return true, nil
}
