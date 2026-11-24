package video

import (
	"context"
	"io"
	"slices"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// Thumbnail は、このツールで上げた動画のサムネイルを設定する。
type Thumbnail struct {
	Remote usecase.Remote
	// Open は、手元のファイルを開き、大きさとともに返す。ファイルがなければ fs.ErrNotExist を包んだエラーを返す。
	Open func(path string) (io.ReadCloser, int64, error)
}

// ThumbnailInput は、Thumbnail の入力。
type ThumbnailInput struct {
	VideoID string // サムネイルを設定する動画の ID
	Path    string // 画像のファイル（jpg か png、2 MiB まで）
}

// ThumbnailResult は、Thumbnail の結果。
type ThumbnailResult struct {
	VideoID string
	URL     string
}

// Run は、画像の形式と大きさを確かめてから、目印のタグがある動画（このツールで上げたもの）にだけ、サムネイルを設定する。
// 目印のない動画や、見つからない動画は KindNotFound。YouTube が断ったときは、そのエラーを返す。
// ledger のロックは、Catalog が自分で取る（その間は持たない）。
func (t Thumbnail) Run(ctx context.Context, in ThumbnailInput) (ThumbnailResult, error) {
	img, mime, err := openThumbnail(t.Open, in.Path)
	if err != nil {
		return ThumbnailResult{}, err
	}
	defer func() { _ = img.Close() }()

	s, err := t.Remote.Connect(ctx)
	if err != nil {
		return ThumbnailResult{}, err
	}
	mine, err := Catalog{Remote: t.Remote}.Videos(ctx, s)
	if err != nil {
		return ThumbnailResult{}, err
	}
	i := slices.IndexFunc(mine, func(v domain.Video) bool { return v.ID == in.VideoID })
	if i < 0 {
		return ThumbnailResult{}, domain.Errorf(domain.KindNotFound, "このツールで上げた動画に見つかりません: %s", in.VideoID)
	}
	if err := s.YT.SetThumbnail(ctx, in.VideoID, img, mime); err != nil {
		return ThumbnailResult{}, err
	}
	return ThumbnailResult{VideoID: in.VideoID, URL: mine[i].URL()}, nil
}
