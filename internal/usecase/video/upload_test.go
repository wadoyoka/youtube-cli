package video_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
	"github.com/wadoyoka/youtube-cli/internal/usecase/video"
)

// clip は、テストで使う手元のファイル。
var clip = usecasetest.Files{"/v/intro.mp4": "VIDEO", "/v/thumb.png": "PNG", "/v/thumb.gif": "GIF"}

func newUpload(yt *usecasetest.YouTube, ledger *usecasetest.Ledger) (video.Upload, *usecasetest.Locker) {
	locker := &usecasetest.Locker{}
	r := usecase.Remote{Open: usecasetest.Opener(yt), State: &usecasetest.State{Saved: testChannel, Found: true}, Ledger: ledger, Locker: locker}
	return video.Upload{Remote: r, Open: clip.Open}, locker
}

// cliVideo は、このツールで上げた、状態が status の動画。
func cliVideo(id, title, status string) domain.Video {
	return domain.Video{ID: id, Title: title, Tags: []string{"go", dv.MarkerTag}, PrivacyStatus: dv.PrivacyUnlisted, UploadStatus: status}
}

// listed は、アップロード済みの再生リストに vs が並んでいる偽の YouTube を返す。
func listed(vs ...domain.Video) *usecasetest.YouTube {
	yt := &usecasetest.YouTube{Channel: testChannel, Videos: vs, PlaylistVideoIDs: map[string][]string{}}
	for _, v := range vs {
		yt.PlaylistVideoIDs[testChannel.UploadsPlaylistID] = append(yt.PlaylistVideoIDs[testChannel.UploadsPlaylistID], v.ID)
	}
	return yt
}

func count(calls []string, name string) int {
	n := 0
	for _, c := range calls {
		if c == name {
			n++
		}
	}
	return n
}

func TestUpload(t *testing.T) {
	ctx := context.Background()

	t.Run("タイトルを省くと拡張子を除いたファイル名にし、目印のタグと既定のカテゴリを付けて上げる", func(t *testing.T) {
		// Arrange
		yt := listed()
		yt.InsertResult = cliVideo("new1", "intro", "uploaded")
		u, _ := newUpload(yt, &usecasetest.Ledger{})
		var reported []int64

		// Act
		res, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4", Meta: dv.Meta{Tags: []string{"go"}}, Progress: func(n int64) { reported = append(reported, n) }})

		// Assert
		if err != nil || len(yt.Inserted) != 1 {
			t.Fatalf("err=%v inserted=%v", err, yt.Inserted)
		}
		ins := yt.Inserted[0]
		if ins.Meta.Title != "intro" || !slices.Equal(ins.Meta.Tags, []string{"go", dv.MarkerTag}) || ins.Meta.CategoryID != dv.DefaultCategory {
			t.Errorf("meta=%+v", ins.Meta)
		}
		if ins.Body != "VIDEO" || ins.Size != 5 || !slices.Equal(reported, []int64{5}) {
			t.Errorf("body=%q size=%d reported=%v", ins.Body, ins.Size, reported)
		}
		if res.Video.ID != "new1" || res.URL != "https://youtu.be/new1" || res.StudioURL != "https://studio.youtube.com/video/new1/edit" || res.Existing {
			t.Errorf("res=%+v", res)
		}
		if len(res.Warnings) != 0 || res.Thumbnail != (video.StepResult{}) || count(yt.Calls, "SetThumbnail") != 0 {
			t.Errorf("res=%+v calls=%v", res, yt.Calls)
		}
	})

	t.Run("上げる前の確かめで失敗したら InsertVideo を呼ばない", func(t *testing.T) {
		longTags := []string{strings.Repeat("a", 500)}
		if err := dv.ValidateMeta(dv.Meta{Title: "t", Tags: longTags}); err != nil {
			t.Fatalf("目印のないタグだけなら上限に収まるはず: %v", err)
		}
		cases := []struct {
			name string
			in   video.UploadInput
			want domain.ErrorKind
		}{
			{"ファイルがないと KindNotFound", video.UploadInput{Path: "/v/none.mp4"}, domain.KindNotFound},
			{"タイトルが長いと KindUsage", video.UploadInput{Path: "/v/intro.mp4", Meta: dv.Meta{Title: strings.Repeat("あ", 101)}}, domain.KindUsage},
			{"目印を足すとタグの合計が 500 文字を超えると KindUsage", video.UploadInput{Path: "/v/intro.mp4", Meta: dv.Meta{Tags: longTags}}, domain.KindUsage},
			{"サムネイルが gif だと KindUsage", video.UploadInput{Path: "/v/intro.mp4", ThumbnailPath: "/v/thumb.gif"}, domain.KindUsage},
			{"サムネイルがないと KindNotFound", video.UploadInput{Path: "/v/intro.mp4", ThumbnailPath: "/v/none.png"}, domain.KindNotFound},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				// Arrange
				yt := listed()
				yt.InsertResult = cliVideo("new1", "intro", "uploaded")
				u, _ := newUpload(yt, &usecasetest.Ledger{})

				// Act
				_, err := u.Run(ctx, tc.in)

				// Assert
				if domain.KindOf(err) != tc.want {
					t.Errorf("err=%v kind=%v want=%v", err, domain.KindOf(err), tc.want)
				}
				if count(yt.Calls, "InsertVideo") != 0 {
					t.Errorf("calls=%v", yt.Calls)
				}
			})
		}
	})

	t.Run("同じタイトルの CLI 動画（processed）があれば KindConflict で、InsertVideo を呼ばない", func(t *testing.T) {
		// Arrange
		yt := listed(cliVideo("old1", "intro", "processed"))
		u, _ := newUpload(yt, &usecasetest.Ledger{})

		// Act
		_, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4"})

		// Assert
		if domain.KindOf(err) != domain.KindConflict || count(yt.Calls, "InsertVideo") != 0 {
			t.Fatalf("err=%v calls=%v", err, yt.Calls)
		}
		if de, _ := errors.AsType[*domain.Error](err); !strings.Contains(strings.Join(de.Details, "\n"), "old1") || !strings.Contains(err.Error(), "--skip-existing") {
			t.Errorf("既存の動画と --skip-existing を案内するはず: %v %v", err, de.Details)
		}
	})

	t.Run("ledger にだけある同じタイトルの動画でも KindConflict", func(t *testing.T) {
		// Arrange
		yt := &usecasetest.YouTube{Channel: testChannel, Videos: []domain.Video{cliVideo("old2", "intro", "uploaded")}}
		ledger := &usecasetest.Ledger{VideoList: []usecase.LedgerVideo{{ID: "old2", Title: "intro"}}}
		u, _ := newUpload(yt, ledger)

		// Act
		_, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4"})

		// Assert
		if domain.KindOf(err) != domain.KindConflict || count(yt.Calls, "InsertVideo") != 0 {
			t.Errorf("err=%v calls=%v", err, yt.Calls)
		}
	})

	t.Run("SkipExisting で processed の既存動画があれば、上げずにサムネイルを設定し、Existing=true", func(t *testing.T) {
		// Arrange
		yt := listed(cliVideo("old1", "intro", "processed"))
		ledger := &usecasetest.Ledger{}
		u, _ := newUpload(yt, ledger)

		// Act
		res, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4", ThumbnailPath: "/v/thumb.png", SkipExisting: true})

		// Assert
		if err != nil || !res.Existing || res.Video.ID != "old1" || res.URL != "https://youtu.be/old1" || !res.Thumbnail.Done {
			t.Fatalf("res=%+v err=%v", res, err)
		}
		if count(yt.Calls, "InsertVideo") != 0 || len(ledger.VideoList) != 0 {
			t.Errorf("calls=%v ledger=%v", yt.Calls, ledger.VideoList)
		}
		want := []usecasetest.SetThumbnailCall{{VideoID: "old1", Image: "PNG", MimeType: "image/png"}}
		if !slices.Equal(yt.Thumbnails, want) {
			t.Errorf("thumbnails=%v", yt.Thumbnails)
		}
	})

	t.Run("SkipExisting で処理中（uploaded）の既存動画があれば、上げずにそれを返す", func(t *testing.T) {
		// Arrange
		yt := listed(cliVideo("old1", "intro", "uploaded"))
		u, _ := newUpload(yt, &usecasetest.Ledger{})

		// Act
		res, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4", SkipExisting: true})

		// Assert
		if err != nil || !res.Existing || res.Video.ID != "old1" || count(yt.Calls, "InsertVideo") != 0 || count(yt.Calls, "SetThumbnail") != 0 {
			t.Errorf("res=%+v err=%v calls=%v", res, err, yt.Calls)
		}
	})

	t.Run("SkipExisting で同じタイトルの動画が 2 本あれば、いちばん新しいものを使い、警告を入れる", func(t *testing.T) {
		// Arrange
		older, newer := cliVideo("old1", "intro", "processed"), cliVideo("old2", "intro", "processed")
		older.PublishedAt, newer.PublishedAt = at(1), at(2)
		yt := listed(older, newer)
		u, _ := newUpload(yt, &usecasetest.Ledger{})

		// Act
		res, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4", SkipExisting: true})

		// Assert
		if err != nil || res.Video.ID != "old2" || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "2 本") {
			t.Errorf("res=%+v err=%v", res, err)
		}
	})

	t.Run("SkipExisting でも既存が failed なら KindConflict と Studio で消す案内", func(t *testing.T) {
		// Arrange
		yt := listed(cliVideo("bad1", "intro", "failed"))
		u, _ := newUpload(yt, &usecasetest.Ledger{})

		// Act
		_, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4", ThumbnailPath: "/v/thumb.png", SkipExisting: true})

		// Assert
		if domain.KindOf(err) != domain.KindConflict || !strings.Contains(err.Error(), "Studio") || !strings.Contains(err.Error(), "消して") {
			t.Fatalf("err=%v", err)
		}
		if de, _ := errors.AsType[*domain.Error](err); !strings.Contains(strings.Join(de.Details, "\n"), "https://studio.youtube.com/video/bad1/edit") {
			t.Errorf("details=%v", de.Details)
		}
		if count(yt.Calls, "InsertVideo") != 0 || count(yt.Calls, "SetThumbnail") != 0 {
			t.Errorf("calls=%v", yt.Calls)
		}
	})

	t.Run("ErrUploadUncertain が返ったら、送り直さずにそのエラーを返す（InsertVideo は1回だけ）", func(t *testing.T) {
		// Arrange
		yt := listed()
		yt.InsertErr = domain.UploadUncertain(errors.New("接続が切れた"))
		ledger := &usecasetest.Ledger{}
		u, _ := newUpload(yt, ledger)

		// Act
		_, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4", ThumbnailPath: "/v/thumb.png"})

		// Assert
		if !errors.Is(err, domain.ErrUploadUncertain) || domain.KindOf(err).ExitCode() != 1 {
			t.Fatalf("err=%v", err)
		}
		if count(yt.Calls, "InsertVideo") != 1 || count(yt.Calls, "SetThumbnail") != 0 || len(ledger.VideoList) != 0 {
			t.Errorf("calls=%v ledger=%v", yt.Calls, ledger.VideoList)
		}
	})

	t.Run("動画 ID を受け取ったら ledger に足す", func(t *testing.T) {
		// Arrange
		yt := listed()
		yt.InsertResult = cliVideo("new1", "題", "uploaded")
		ledger := &usecasetest.Ledger{}
		u, locker := newUpload(yt, ledger)

		// Act
		_, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4", Meta: dv.Meta{Title: "題"}})

		// Assert
		if err != nil || len(ledger.VideoList) != 1 {
			t.Fatalf("err=%v ledger=%v", err, ledger.VideoList)
		}
		got := ledger.VideoList[0]
		if got.ID != "new1" || got.Title != "題" || got.Size != 5 || got.UploadedAt.IsZero() {
			t.Errorf("ledger=%+v", got)
		}
		if locker.Locks != 2 {
			t.Errorf("一覧を引くときと記録するときにロックするはず: locks=%d", locker.Locks)
		}
	})

	t.Run("ledger に書けなくても、上げた動画を返し、警告を入れる", func(t *testing.T) {
		// Arrange
		yt := listed()
		yt.InsertResult = cliVideo("new1", "intro", "uploaded")
		u, _ := newUpload(yt, &usecasetest.Ledger{AddErr: errors.New("書けない")})

		// Act
		res, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4"})

		// Assert
		if err != nil || res.Video.ID != "new1" || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "書けない") {
			t.Errorf("res=%+v err=%v", res, err)
		}
	})

	t.Run("返った privacyStatus が private なら警告が入り、結果にも private", func(t *testing.T) {
		// Arrange
		yt := listed()
		yt.InsertResult = domain.Video{ID: "new1", Title: "intro", Tags: []string{dv.MarkerTag}, PrivacyStatus: "private", UploadStatus: "uploaded"}
		u, _ := newUpload(yt, &usecasetest.Ledger{})

		// Act
		res, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4"})

		// Assert
		if err != nil || res.Video.PrivacyStatus != "private" {
			t.Fatalf("res=%+v err=%v", res, err)
		}
		if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "private") {
			t.Errorf("warnings=%v", res.Warnings)
		}
	})

	t.Run("サムネイルを指定すると、上げたあとで MIME タイプを付けて設定する", func(t *testing.T) {
		// Arrange
		yt := listed()
		yt.InsertResult = cliVideo("new1", "intro", "uploaded")
		u, _ := newUpload(yt, &usecasetest.Ledger{})

		// Act
		res, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4", ThumbnailPath: "/v/thumb.png"})

		// Assert
		if err != nil || res.Thumbnail != (video.StepResult{Done: true}) {
			t.Fatalf("res=%+v err=%v", res, err)
		}
		want := []usecasetest.SetThumbnailCall{{VideoID: "new1", Image: "PNG", MimeType: "image/png"}}
		if !slices.Equal(yt.Thumbnails, want) || !slices.Equal(yt.Calls[len(yt.Calls)-2:], []string{"InsertVideo", "SetThumbnail"}) {
			t.Errorf("thumbnails=%v calls=%v", yt.Thumbnails, yt.Calls)
		}
	})

	t.Run("サムネイルの設定が失敗したら、動画の結果を返しつつ終了コード 1 になるエラー", func(t *testing.T) {
		// Arrange
		yt := listed()
		yt.InsertResult = cliVideo("new1", "intro", "uploaded")
		yt.ThumbnailErr = domain.Errorf(domain.KindQuota, "クォータ切れ")
		ledger := &usecasetest.Ledger{}
		u, _ := newUpload(yt, ledger)

		// Act
		res, err := u.Run(ctx, video.UploadInput{Path: "/v/intro.mp4", ThumbnailPath: "/v/thumb.png"})

		// Assert
		if err == nil || domain.KindOf(err).ExitCode() != 1 || !strings.Contains(err.Error(), "--skip-existing") {
			t.Fatalf("err=%v", err)
		}
		if res.Video.ID != "new1" || res.Thumbnail.Done || !strings.Contains(res.Thumbnail.Error, "クォータ切れ") || len(ledger.VideoList) != 1 {
			t.Errorf("res=%+v ledger=%v", res, ledger.VideoList)
		}
	})
}
