package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
	"github.com/wadoyoka/youtube-cli/internal/usecase/video"
)

func withVideos(d Deps, vs ...domain.Video) Deps {
	var idsList []string
	for _, v := range vs {
		idsList = append(idsList, v.ID)
	}
	yt := &usecasetest.YouTube{
		Channel:          testChannel,
		PlaylistVideoIDs: map[string][]string{testChannel.UploadsPlaylistID: idsList},
		Videos:           vs,
	}
	d.List = video.List{Remote: usecase.Remote{
		Open:   usecasetest.Opener(yt),
		State:  &usecasetest.State{Saved: testChannel, Found: true},
		Locker: &usecasetest.Locker{},
		Ledger: &usecasetest.Ledger{},
	}}
	return d
}

func TestLsCommand(t *testing.T) {
	published := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tagged := domain.Video{ID: "v1", Title: "動画1", Tags: []string{"memo", dv.MarkerTag}, PrivacyStatus: "unlisted", UploadStatus: "processed", PublishedAt: published}
	untagged := domain.Video{ID: "v2", Title: "動画2", PrivacyStatus: "unlisted", UploadStatus: "processed", PublishedAt: published}

	t.Run("--json は id・title・url などの配列を出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withVideos(d, tagged)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"--json", "ls"})

		// Assert
		var got []map[string]any
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("json: %v: %q", err, out.String())
		}
		if code != 0 || len(got) != 1 {
			t.Fatalf("code=%d got=%v", code, got)
		}
		v := got[0]
		for k, want := range map[string]any{"id": "v1", "title": "動画1", "url": "https://youtu.be/v1", "privacyStatus": "unlisted", "uploadStatus": "processed", "publishedAt": "2026-01-02T03:04:05Z"} {
			if v[k] != want {
				t.Errorf("%s = %v, want %v", k, v[k], want)
			}
		}
		if tags, ok := v["tags"].([]any); !ok || len(tags) != 2 {
			t.Errorf("tags = %v", v["tags"])
		}
	})

	t.Run("--json は、目印だけのタグでも tags を配列で出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withVideos(d, domain.Video{ID: "v3", Title: "動画3", Tags: []string{dv.MarkerTag}})
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"--json", "ls"})

		// Assert
		if code != 0 || !strings.Contains(out.String(), `"tags":["youtube-cli"]`) {
			t.Errorf("code=%d out=%q", code, out.String())
		}
	})

	t.Run("--json は、動画がなくても [] を出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withVideos(d, untagged)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"--json", "ls"})

		// Assert
		if code != 0 || strings.TrimSpace(out.String()) != "[]" {
			t.Errorf("code=%d out=%q", code, out.String())
		}
	})

	t.Run("人向けの出力は、タイトル・URL・状態を1行に出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withVideos(d, tagged)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"ls"})

		// Assert
		want := "動画1\thttps://youtu.be/v1\tunlisted/processed\n"
		if code != 0 || out.String() != want {
			t.Errorf("code=%d out=%q want=%q", code, out.String(), want)
		}
	})

	t.Run("-l は ID・公開日時・タグを足す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withVideos(d, tagged)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"ls", "-l"})

		// Assert
		want := "v1\t動画1\thttps://youtu.be/v1\tunlisted/processed\t2026-01-02T03:04:05Z\tmemo,youtube-cli\n"
		if code != 0 || out.String() != want {
			t.Errorf("code=%d out=%q want=%q", code, out.String(), want)
		}
	})
}
