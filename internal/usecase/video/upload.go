package video

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"time"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// Upload は、手元の動画を限定公開で上げる。
type Upload struct {
	Remote usecase.Remote
	// Open は、手元のファイルを開き、大きさとともに返す。ファイルがなければ fs.ErrNotExist を包んだエラーを返す。
	Open func(path string) (io.ReadCloser, int64, error)
}

// UploadInput は、Upload の入力。
type UploadInput struct {
	Path string // 上げる動画のファイル
	// Meta は、動画の項目。Title が空ならファイル名（拡張子を除く）、CategoryID が空なら既定のカテゴリにする。目印のタグは足す。
	Meta          dv.Meta
	ThumbnailPath string           // 上げたあとで設定するサムネイル。空なら設定しない
	SkipExisting  bool             // 同じタイトルの CLI 動画があれば、上げずにそれを使う
	Progress      func(sent int64) // 送り終えたバイト数を知らせる先。nil なら知らせない
}

// StepResult は、上げたあとの手順（サムネイルの設定など）の結果。行わなかったときは空。
type StepResult struct {
	Done  bool
	Error string
}

// UploadResult は、Upload の結果。
type UploadResult struct {
	Video     domain.Video
	URL       string
	StudioURL string
	Existing  bool // 上げずに、同じタイトルの既存の動画を使った
	Thumbnail StepResult
	Warnings  []string
}

// Run は、上げる前にすべてを確かめてから、動画を限定公開で上げ、サムネイルを設定する。
//  1. 項目（目印のタグを足したもの）、動画のファイル、サムネイルの形式と大きさを確かめる。失敗すれば何も上げない
//  2. 同じタイトルの CLI 動画があれば KindConflict で止める。SkipExisting なら、使える既存の動画で残りの手順だけを行う
//  3. 上げる。結果が分からない（domain.ErrUploadUncertain）ときは、送り直さずにそのまま返す
//  4. 動画 ID を受け取ったら、すぐに ledger に記録する。記録できなくても、上げた動画は警告を付けて返す
//  5. 限定公開で返ってこなければ警告する
//  6. サムネイルを設定する。失敗しても動画は残るので、結果を返しつつ KindUnexpected のエラーにする
//
// ledger のロックは、記録するときだけ取る（Catalog が自分でロックを取るので、その間は持たない）。
func (u Upload) Run(ctx context.Context, in UploadInput) (UploadResult, error) {
	meta := in.Meta
	if meta.Title == "" {
		meta.Title = dv.TitleFromPath(in.Path)
	}
	if meta.CategoryID == "" {
		meta.CategoryID = dv.DefaultCategory
	}
	meta.Tags = dv.WithMarker(meta.Tags)
	if err := dv.ValidateMeta(meta); err != nil {
		return UploadResult{}, err
	}
	body, size, err := u.open(in.Path, "動画")
	if err != nil {
		return UploadResult{}, err
	}
	defer func() { _ = body.Close() }()
	var thumb io.ReadCloser
	var thumbMime string
	if in.ThumbnailPath != "" {
		if thumb, thumbMime, err = u.openThumbnail(in.ThumbnailPath); err != nil {
			return UploadResult{}, err
		}
		defer func() { _ = thumb.Close() }()
	}

	s, err := u.Remote.Connect(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	existing, warnings, err := u.sameTitle(ctx, s, meta.Title, in.SkipExisting)
	if err != nil {
		return UploadResult{}, err
	}
	res := UploadResult{Warnings: warnings}
	if existing != nil {
		res.Video, res.Existing = *existing, true
	} else {
		v, err := s.YT.InsertVideo(ctx, meta, body, size, in.Progress)
		if err != nil {
			return UploadResult{}, err
		}
		res.Video = v
		if err := u.record(ctx, meta.Title, v, size); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("上げた動画を手元の記録に足せませんでした（しばらくの間、同じタイトルの確かめや ls に出ないことがあります）: %v", err))
		}
		if v.PrivacyStatus != dv.PrivacyUnlisted {
			res.Warnings = append(res.Warnings, fmt.Sprintf("YouTube が限定公開（%s）ではなく %q で返しました。Studio で公開範囲を確かめてください", dv.PrivacyUnlisted, v.PrivacyStatus))
		}
	}
	res.URL, res.StudioURL = res.Video.URL(), res.Video.StudioURL()

	if thumb != nil {
		if err := s.YT.SetThumbnail(ctx, res.Video.ID, thumb, thumbMime); err != nil {
			res.Thumbnail.Error = err.Error()
			return res, domain.Errorf(domain.KindUnexpected,
				"サムネイルを設定できませんでした（動画 %s は YouTube にあります）。同じコマンドに --skip-existing を付けてやり直すと、サムネイルの設定だけを行います", res.Video.ID).Wrap(err)
		}
		res.Thumbnail.Done = true
	}
	return res, nil
}

// open は、手元のファイルを開く。ファイルがなければ KindNotFound、開けなければ KindUsage。what は「動画」などの呼び名。
func (u Upload) open(path, what string) (io.ReadCloser, int64, error) {
	f, size, err := u.Open(path)
	switch {
	case err == nil:
		return f, size, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, 0, domain.Errorf(domain.KindNotFound, "%sのファイルが見つかりません: %s", what, path).Wrap(err)
	default:
		return nil, 0, domain.Errorf(domain.KindUsage, "%sのファイルを開けません: %s", what, path).Wrap(err)
	}
}

// openThumbnail は、サムネイルを開き、形式と大きさを確かめて、MIME タイプとともに返す。
func (u Upload) openThumbnail(path string) (io.ReadCloser, string, error) {
	f, size, err := u.open(path, "サムネイル")
	if err != nil {
		return nil, "", err
	}
	mime, err := dv.ValidateThumbnail(path, size)
	if err != nil {
		_ = f.Close()
		return nil, "", err
	}
	return f, mime, nil
}

// sameTitle は、同じタイトルの CLI 動画を確かめる。なければ nil を返し、上げてよい。
// 使えない状態（失敗・却下・削除）のものがあれば、Studio で消すよう案内して KindConflict。
// 使えるものだけなら、skip のときはいちばん新しいものを返し、そうでなければ KindConflict。
func (u Upload) sameTitle(ctx context.Context, s *usecase.Session, title string, skip bool) (*domain.Video, []string, error) {
	same, err := Catalog{Remote: u.Remote}.ByTitle(ctx, s, title)
	if err != nil || len(same) == 0 {
		return nil, nil, err
	}
	details := make([]string, 0, len(same))
	for _, v := range same {
		details = append(details, fmt.Sprintf("%s（%s）%s", v.ID, v.UploadStatus, v.StudioURL()))
	}
	if slices.ContainsFunc(same, func(v domain.Video) bool { return dv.Unusable(v.UploadStatus) }) {
		return nil, nil, domain.Errorf(domain.KindConflict,
			"同じタイトルの CLI 動画が、使えない状態（失敗・却下・削除）で残っています。Studio でその動画を消してから、やり直してください: %s", title).WithDetails(details...)
	}
	if !skip {
		return nil, nil, domain.Errorf(domain.KindConflict,
			"同じタイトルの CLI 動画がすでにあります: %s。上げずにその動画を使うときは --skip-existing を付けてください", title).WithDetails(details...)
	}
	var warnings []string
	if len(same) > 1 {
		warnings = append(warnings, fmt.Sprintf("同じタイトルの CLI 動画が %d 本あります。いちばん新しい %s を使いました", len(same), same[0].ID))
	}
	return &same[0], warnings, nil
}

// record は、上げた動画を ledger に記録する。
func (u Upload) record(ctx context.Context, title string, v domain.Video, size int64) error {
	unlock, err := u.Remote.Locker.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return u.Remote.Ledger.AddVideo(usecase.LedgerVideo{ID: v.ID, Title: title, Size: size, UploadedAt: time.Now()})
}
