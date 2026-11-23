package video

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/playlist"
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
	PlaylistRef   string           // 上げたあとで入れる、このツールで作った再生リスト（ID か名前）。空なら入れない
	SkipExisting  bool             // 同じタイトルの CLI 動画があれば、上げずにそれを使う
	Progress      func(sent int64) // 送り終えたバイト数を知らせる先。nil なら知らせない
}

// StepResult は、上げたあとの手順（サムネイルの設定、再生リストへの追加）の結果。行わなかったときは空。
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
	Playlist  StepResult // すでに入っていて何もしなかったときも Done
	Warnings  []string
}

// Run は、上げる前にすべてを確かめてから、動画を限定公開で上げ、サムネイルを設定し、再生リストに入れる。
//  1. 項目（目印のタグを足したもの）、動画のファイル、サムネイルの形式と大きさを確かめる。失敗すれば何も上げない
//  2. 再生リストが、このツールで作った限定公開のものかを確かめる（playlist.Catalog.ResolveForAdd）。失敗すれば何も上げない
//  3. 同じタイトルの CLI 動画があれば KindConflict で止める。SkipExisting なら、使える既存の動画で残りの手順だけを行う
//  4. 上げる。結果が分からない（domain.ErrUploadUncertain）ときは、送り直さずにそのまま返す
//  5. 動画 ID を受け取ったら、すぐに ledger に記録する。記録できなくても、上げた動画は警告を付けて返す
//  6. 限定公開で返ってこなければ警告する
//  7. サムネイルを設定し、再生リストに入れる（すでに入っていれば何もしない）。片方が失敗してももう片方は行う。
//     失敗しても動画は残るので、結果を返しつつ KindUnexpected のエラーにする。5 で記録できなかったときは、
//     --skip-existing ではなく、失敗した手順だけのコマンド（thumbnail、playlist add）でのやり直しを案内する
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
	body, size, err := openLocal(u.Open, in.Path, "動画")
	if err != nil {
		return UploadResult{}, err
	}
	defer func() { _ = body.Close() }()
	var thumb io.ReadCloser
	var thumbMime string
	if in.ThumbnailPath != "" {
		if thumb, thumbMime, err = openThumbnail(u.Open, in.ThumbnailPath); err != nil {
			return UploadResult{}, err
		}
		defer func() { _ = thumb.Close() }()
	}

	s, err := u.Remote.Connect(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	var playlistID string // 上げたあとで入れる再生リスト。空なら入れない
	if in.PlaylistRef != "" {
		p, err := playlist.Catalog{Remote: u.Remote}.ResolveForAdd(ctx, s, in.PlaylistRef)
		if err != nil {
			return UploadResult{}, err
		}
		playlistID = p.ID
	}
	existing, warnings, err := u.sameTitle(ctx, s, meta.Title, in.SkipExisting)
	if err != nil {
		return UploadResult{}, err
	}
	res := UploadResult{Warnings: warnings}
	recorded := true // 同じタイトルの確かめで、この動画が見つかる。false なら、YouTube の一覧に出るまで見つからない
	if existing != nil {
		res.Video, res.Existing = *existing, true
	} else {
		v, err := s.YT.InsertVideo(ctx, meta, body, size, in.Progress)
		if err != nil {
			return UploadResult{}, err
		}
		res.Video = v
		if err := u.record(ctx, meta.Title, v, size); err != nil {
			recorded = false
			res.Warnings = append(res.Warnings, fmt.Sprintf("上げた動画を手元の記録に足せませんでした（しばらくの間、同じタイトルの確かめや ls に出ないことがあります）: %v", err))
		}
		if v.PrivacyStatus != dv.PrivacyUnlisted {
			res.Warnings = append(res.Warnings, fmt.Sprintf("YouTube が限定公開（%s）ではなく %q で返しました。Studio で公開範囲を確かめてください", dv.PrivacyUnlisted, v.PrivacyStatus))
		}
	}
	res.URL, res.StudioURL = res.Video.URL(), res.Video.StudioURL()

	var failed, redo []string // 失敗した手順の名前と、その手順だけをやり直すコマンド
	var errs []error
	step := func(name, cmd string, err error) StepResult {
		if err != nil {
			failed, redo, errs = append(failed, name), append(redo, cmd), append(errs, err)
			return StepResult{Error: err.Error()}
		}
		return StepResult{Done: true}
	}
	if thumb != nil {
		res.Thumbnail = step("サムネイルの設定", fmt.Sprintf("`youtube-cli thumbnail %s <画像>`", res.Video.ID),
			s.YT.SetThumbnail(ctx, res.Video.ID, thumb, thumbMime))
	}
	if playlistID != "" {
		_, err := playlist.AddIfAbsent(ctx, s.YT, playlistID, res.Video.ID)
		res.Playlist = step("再生リストへの追加", fmt.Sprintf("`youtube-cli playlist add %s %s`", res.Video.ID, playlistID), err)
	}
	if len(failed) > 0 {
		how := "同じコマンドに --skip-existing を付けてやり直すと、上げずに残りの手順だけを行います"
		if !recorded {
			// 手元の記録がないと、--skip-existing は一覧に出る前のこの動画を見つけられず、もう 1 本上げてしまう。
			// thumbnail と playlist add は上げ直さない（一覧に出る前なら、見つからずに止まるだけ）。
			how = "手元の記録に足せなかったため、同じコマンドに --skip-existing を付けてやり直すと、もう1本上がることがあります。" +
				strings.Join(redo, "、") + " で、失敗した手順だけをやり直してください（見つからないと出たら、`youtube-cli ls --json` に出るまで待ってから）"
		}
		return res, domain.Errorf(domain.KindUnexpected, "%sに失敗しました（動画 %s は YouTube にあります）。%s",
			strings.Join(failed, "と"), res.Video.ID, how).Wrap(errors.Join(errs...))
	}
	return res, nil
}

// openLocal は、open で手元のファイルを開く。ファイルがなければ KindNotFound、開けなければ KindUsage。what は「動画」などの呼び名。
func openLocal(open func(string) (io.ReadCloser, int64, error), path, what string) (io.ReadCloser, int64, error) {
	f, size, err := open(path)
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
func openThumbnail(open func(string) (io.ReadCloser, int64, error), path string) (io.ReadCloser, string, error) {
	f, size, err := openLocal(open, path, "サムネイル")
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
