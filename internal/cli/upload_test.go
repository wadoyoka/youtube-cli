package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
	"github.com/wadoyoka/youtube-cli/internal/usecase/video"
)

// uploadFiles は、upload のテストで使う手元のファイル。
var uploadFiles = usecasetest.Files{"/v/intro.mp4": "VIDEO", "/v/thumb.png": "PNG"}

// withUpload は、偽の YouTube に上げる Upload を d に入れる。上げると、ID が new1 の動画が返る。
func withUpload(d Deps) (Deps, *usecasetest.YouTube) {
	yt := &usecasetest.YouTube{
		Channel:      testChannel,
		InsertResult: domain.Video{ID: "new1", Title: "intro", Tags: []string{dv.MarkerTag}, PrivacyStatus: dv.PrivacyUnlisted, UploadStatus: "uploaded"},
	}
	d.Upload = video.Upload{Remote: usecase.Remote{
		Open:   usecasetest.Opener(yt),
		State:  &usecasetest.State{Saved: testChannel, Found: true},
		Locker: &usecasetest.Locker{},
		Ledger: &usecasetest.Ledger{},
	}, Open: uploadFiles.Open}
	return d, yt
}

func TestUploadCommand(t *testing.T) {
	ctx := context.Background()

	t.Run("--json は videoId・url・studioUrl・privacyStatus などを出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, _ = withUpload(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"--json", "upload", "/v/intro.mp4"})

		// Assert
		var got map[string]any
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("json: %v: %q (stderr %q)", err, out.String(), errb.String())
		}
		if code != 0 {
			t.Fatalf("code=%d stderr=%q", code, errb.String())
		}
		for k, want := range map[string]any{
			"videoId": "new1", "title": "intro", "url": "https://youtu.be/new1",
			"studioUrl": "https://studio.youtube.com/video/new1/edit", "privacyStatus": "unlisted",
			"uploadStatus": "uploaded", "existing": false, "thumbnail": nil, "playlist": nil,
		} {
			if v, ok := got[k]; !ok || v != want {
				t.Errorf("%s = %v（あるか: %v）, want %v", k, v, ok, want)
			}
		}
		if w, ok := got["warnings"].([]any); !ok || len(w) != 0 {
			t.Errorf("warnings = %v", got["warnings"])
		}
	})

	t.Run("フラグを動画の項目に渡す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withUpload(d)
		initAndClear(t, d, out, errb)
		descFile := filepath.Join(t.TempDir(), "desc.txt")
		if err := os.WriteFile(descFile, []byte("説明です\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		code := Execute(ctx, d, []string{
			"upload", "/v/intro.mp4", "--title", "題", "--description-file", descFile,
			"--tag", "go", "--tag", "a,b", "--category", "27", "--made-for-kids", "--synthetic-media",
		})

		// Assert
		if code != 0 || len(yt.Inserted) != 1 {
			t.Fatalf("code=%d stderr=%q", code, errb.String())
		}
		m := yt.Inserted[0].Meta
		if m.Title != "題" || m.Description != "説明です\n" || strings.Join(m.Tags, "|") != "go|a,b|youtube-cli" ||
			m.CategoryID != "27" || !m.MadeForKids || !m.SyntheticMedia {
			t.Errorf("meta=%+v", m)
		}
	})

	t.Run("フラグを省くと、説明は空、カテゴリは 22、申告はしない", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withUpload(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"upload", "/v/intro.mp4", "--description", "短い説明"})

		// Assert
		if code != 0 || len(yt.Inserted) != 1 {
			t.Fatalf("code=%d stderr=%q", code, errb.String())
		}
		m := yt.Inserted[0].Meta
		if m.Title != "intro" || m.Description != "短い説明" || m.CategoryID != "22" || m.MadeForKids || m.SyntheticMedia {
			t.Errorf("meta=%+v", m)
		}
	})

	t.Run("--description と --description-file を同時に指定すると 2 で、上げない", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withUpload(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"upload", "/v/intro.mp4", "--description", "a", "--description-file", "/v/desc.txt"})

		// Assert
		if code != 2 || len(yt.Inserted) != 0 || out.Len() != 0 {
			t.Errorf("code=%d inserted=%d out=%q", code, len(yt.Inserted), out.String())
		}
	})

	t.Run("説明のファイルがなければ 4 で、上げない", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withUpload(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"upload", "/v/intro.mp4", "--description-file", filepath.Join(t.TempDir(), "none.txt")})

		// Assert
		if code != 4 || len(yt.Inserted) != 0 {
			t.Errorf("code=%d inserted=%d stderr=%q", code, len(yt.Inserted), errb.String())
		}
	})

	t.Run("動画を指定しなければ 2", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, _ = withUpload(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"upload"})

		// Assert
		if code != 2 {
			t.Errorf("code=%d", code)
		}
	})

	t.Run("--thumbnail を付けると thumbnail も拒否リストで確かめ、拒否なら 7 で止まって上げない", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, yt := withUpload(d)
		initAndClear(t, d, out, errb)
		if err := os.WriteFile(filepath.Join(dir, "policy.toml"), []byte(`deny = ["thumbnail"]`), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		code := Execute(ctx, d, []string{"upload", "/v/intro.mp4", "--thumbnail", "/v/thumb.png"})

		// Assert
		if code != 7 || len(yt.Inserted) != 0 || !strings.Contains(errb.String(), "thumbnail") {
			t.Errorf("code=%d inserted=%d stderr=%q", code, len(yt.Inserted), errb.String())
		}
	})

	t.Run("--thumbnail がなければ、thumbnail を拒否していても上げる", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, yt := withUpload(d)
		initAndClear(t, d, out, errb)
		if err := os.WriteFile(filepath.Join(dir, "policy.toml"), []byte(`deny = ["thumbnail"]`), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		code := Execute(ctx, d, []string{"upload", "/v/intro.mp4"})

		// Assert
		if code != 0 || len(yt.Inserted) != 1 {
			t.Errorf("code=%d inserted=%d stderr=%q", code, len(yt.Inserted), errb.String())
		}
	})

	t.Run("既定の policy.toml では --playlist を付けると playlist-add で 7 になり、上げない", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withUpload(d)
		yt.Playlists = []domain.Playlist{{ID: "PL1", Title: "Go 入門", Description: dv.AddPlaylistMarker(""), PrivacyStatus: dv.PrivacyUnlisted}}
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"upload", "/v/intro.mp4", "--playlist", "Go 入門"})

		// Assert
		if code != 7 || len(yt.Calls) != 0 || !strings.Contains(errb.String(), "playlist-add") {
			t.Errorf("code=%d calls=%v stderr=%q", code, yt.Calls, errb.String())
		}
	})

	t.Run("playlist-add を許せば、上げたあとで再生リストに入れ、JSON の playlist.done が true", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, yt := withUpload(d)
		yt.Playlists = []domain.Playlist{{ID: "PL1", Title: "Go 入門", Description: dv.AddPlaylistMarker(""), PrivacyStatus: dv.PrivacyUnlisted}}
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"--json", "upload", "/v/intro.mp4", "--playlist", "Go 入門"})

		// Assert
		if code != 0 || !strings.Contains(out.String(), `"playlist":{"done":true}`) || len(yt.AddedToPlaylist) != 1 {
			t.Errorf("code=%d out=%q stderr=%q", code, out.String(), errb.String())
		}
	})

	t.Run("--playlist が公開の再生リストを指すと、上げる前に 7 で止まる", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, yt := withUpload(d)
		yt.Playlists = []domain.Playlist{{ID: "PL1", Title: "Go 入門", Description: dv.AddPlaylistMarker(""), PrivacyStatus: "public"}}
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"upload", "/v/intro.mp4", "--playlist", "Go 入門"})

		// Assert
		if code != 7 || len(yt.Inserted) != 0 || out.Len() != 0 {
			t.Errorf("code=%d inserted=%d out=%q stderr=%q", code, len(yt.Inserted), out.String(), errb.String())
		}
	})

	t.Run("サムネイルを設定したら、JSON の thumbnail.done が true", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, _ = withUpload(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"--json", "upload", "/v/intro.mp4", "--thumbnail", "/v/thumb.png"})

		// Assert
		if code != 0 || !strings.Contains(out.String(), `"thumbnail":{"done":true}`) {
			t.Errorf("code=%d out=%q stderr=%q", code, out.String(), errb.String())
		}
	})

	t.Run("サムネイルに失敗しても結果の JSON を出し、終了コード 1", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withUpload(d)
		yt.ThumbnailErr = errors.New("forbidden")
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"--json", "upload", "/v/intro.mp4", "--thumbnail", "/v/thumb.png"})

		// Assert
		var got struct {
			VideoID   string `json:"videoId"`
			Thumbnail struct {
				Done  bool   `json:"done"`
				Error string `json:"error"`
			} `json:"thumbnail"`
		}
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("json: %v: %q", err, out.String())
		}
		if code != 1 || got.VideoID != "new1" || got.Thumbnail.Done || !strings.Contains(got.Thumbnail.Error, "forbidden") {
			t.Errorf("code=%d got=%+v", code, got)
		}
		if !strings.Contains(errb.String(), `"kind":"unexpected"`) {
			t.Errorf("stderr=%q", errb.String())
		}
	})

	t.Run("上げる前に失敗したら、標準出力には何も出さない", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, _ = withUpload(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"--json", "upload", "/v/none.mp4"})

		// Assert
		if code != 4 || out.Len() != 0 {
			t.Errorf("code=%d out=%q", code, out.String())
		}
	})

	t.Run("結果が分からないときは、Studio で確かめる案内を出して 1", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withUpload(d)
		yt.InsertErr = domain.UploadUncertain(errors.New("接続が切れた"))
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"upload", "/v/intro.mp4"})

		// Assert
		if code != 1 || out.Len() != 0 || !strings.Contains(errb.String(), "Studio で確かめて") || !strings.Contains(errb.String(), "--skip-existing") {
			t.Errorf("code=%d out=%q stderr=%q", code, out.String(), errb.String())
		}
	})

	t.Run("人向けの出力は、上げたかどうか・タイトル・URL・Studio の URL を1行に出し、警告は標準エラーに出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withUpload(d)
		yt.InsertResult.PrivacyStatus = "private"
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"upload", "/v/intro.mp4"})

		// Assert
		want := "uploaded\tintro\thttps://youtu.be/new1\thttps://studio.youtube.com/video/new1/edit\n"
		if code != 0 || out.String() != want {
			t.Errorf("code=%d out=%q want=%q", code, out.String(), want)
		}
		if !strings.Contains(errb.String(), "注意: ") || !strings.Contains(errb.String(), "private") {
			t.Errorf("stderr=%q", errb.String())
		}
	})

	t.Run("--skip-existing で既存の動画を使ったら existing と出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withUpload(d)
		old := domain.Video{ID: "old1", Title: "intro", Tags: []string{dv.MarkerTag}, PrivacyStatus: dv.PrivacyUnlisted, UploadStatus: "processed"}
		yt.Videos = []domain.Video{old}
		yt.PlaylistVideoIDs = map[string][]string{testChannel.UploadsPlaylistID: {"old1"}}
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"upload", "/v/intro.mp4", "--skip-existing"})

		// Assert
		want := "existing\tintro\thttps://youtu.be/old1\thttps://studio.youtube.com/video/old1/edit\n"
		if code != 0 || out.String() != want || len(yt.Inserted) != 0 {
			t.Errorf("code=%d out=%q want=%q stderr=%q", code, out.String(), want, errb.String())
		}
	})
}
