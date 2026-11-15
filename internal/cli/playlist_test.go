package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/playlist"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
	"github.com/wadoyoka/youtube-cli/internal/usecase/video"
)

// cliPlaylist は、このツールで作った限定公開の再生リスト。
func cliPlaylist(id, title string) domain.Playlist {
	return domain.Playlist{ID: id, Title: title, Description: dv.AddPlaylistMarker(""), PrivacyStatus: dv.PrivacyUnlisted, ItemCount: 3}
}

// withPlaylists は、1つの偽の YouTube を共有する playlist の ls・create・add を d に入れる。
// 偽の YouTube には、再生リスト pls と、CLI の動画 v1・CLI でない動画 x1 がある。作ると、ID が PLnew の再生リストが返る。
func withPlaylists(d Deps, pls ...domain.Playlist) (Deps, *usecasetest.YouTube) {
	yt := &usecasetest.YouTube{
		Channel:   testChannel,
		Playlists: pls,
		Videos:    []domain.Video{{ID: "v1", Title: "intro", Tags: []string{dv.MarkerTag}}, {ID: "x1", Title: "他人"}},
		PlaylistVideoIDs: map[string][]string{
			testChannel.UploadsPlaylistID: {"v1", "x1"},
		},
		CreateResult: cliPlaylist("PLnew", "Go 入門"),
	}
	r := usecase.Remote{
		Open:   usecasetest.Opener(yt),
		State:  &usecasetest.State{Saved: testChannel, Found: true},
		Locker: &usecasetest.Locker{},
		Ledger: &usecasetest.Ledger{},
	}
	d.PlaylistList = playlist.List{Remote: r}
	d.PlaylistCreate = playlist.Create{Remote: r}
	d.PlaylistAdd = playlist.Add{Remote: r, Videos: video.Catalog{Remote: r}}
	return d, yt
}

// writePolicy は、policy.toml を deny の内容で書き換える。
func writePolicy(t *testing.T, dir, deny string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "policy.toml"), []byte("deny = ["+deny+"]"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPlaylistPolicy(t *testing.T) {
	ctx := context.Background()

	t.Run("既定の policy.toml では playlist create が 7 で止まり、何も作らない", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withPlaylists(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"playlist", "create", "Go 入門"})

		// Assert
		if code != 7 || len(yt.Calls) != 0 || !strings.Contains(errb.String(), "playlist-create") {
			t.Errorf("code=%d calls=%v stderr=%q", code, yt.Calls, errb.String())
		}
	})

	t.Run("既定の policy.toml では playlist add が 7 で止まり、何も入れない", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, yt := withPlaylists(d, cliPlaylist("PL1", "Go 入門"))
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"playlist", "add", "v1", "PL1"})

		// Assert
		if code != 7 || len(yt.Calls) != 0 || !strings.Contains(errb.String(), "playlist-add") {
			t.Errorf("code=%d calls=%v stderr=%q", code, yt.Calls, errb.String())
		}
	})

	t.Run("既定の policy.toml でも playlist ls は通る", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, _ = withPlaylists(d, cliPlaylist("PL1", "Go 入門"))
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"playlist", "ls"})

		// Assert
		if code != 0 || !strings.Contains(out.String(), "Go 入門") {
			t.Errorf("code=%d out=%q stderr=%q", code, out.String(), errb.String())
		}
	})

	t.Run("ls を拒否しても playlist ls は通る", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, _ = withPlaylists(d, cliPlaylist("PL1", "Go 入門"))
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, `"ls"`)

		// Act
		code := Execute(ctx, d, []string{"playlist", "ls"})

		// Assert
		if code != 0 {
			t.Errorf("code=%d stderr=%q", code, errb.String())
		}
	})

	t.Run("playlist-ls を拒否すると playlist ls は 7 で止まる", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, yt := withPlaylists(d, cliPlaylist("PL1", "Go 入門"))
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, `"playlist-ls"`)

		// Act
		code := Execute(ctx, d, []string{"playlist", "ls"})

		// Assert
		if code != 7 || len(yt.Calls) != 0 {
			t.Errorf("code=%d calls=%v", code, yt.Calls)
		}
	})

	t.Run("playlist だけなら、init の前でも使い方を出して 0", func(t *testing.T) {
		// Arrange
		d, out, _, _ := newDeps(t)

		// Act
		code := Execute(ctx, d, []string{"playlist"})

		// Assert
		if code != 0 || !strings.Contains(out.String(), "create") {
			t.Errorf("code=%d out=%q", code, out.String())
		}
	})

	t.Run("playlist の知らないサブコマンドは 2", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"playlist", "nope"})

		// Assert
		if code != 2 {
			t.Errorf("code=%d stderr=%q", code, errb.String())
		}
	})
}

func TestPlaylistLsCommand(t *testing.T) {
	ctx := context.Background()

	t.Run("--json は id・title・url・privacyStatus・itemCount の配列を出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, _ = withPlaylists(d, cliPlaylist("PL1", "Go 入門"), domain.Playlist{ID: "PLhand", Title: "手で作った"})
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"--json", "playlist", "ls"})

		// Assert
		var got []map[string]any
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || code != 0 || len(got) != 1 {
			t.Fatalf("code=%d err=%v out=%q", code, err, out.String())
		}
		for k, want := range map[string]any{"id": "PL1", "title": "Go 入門", "url": "https://www.youtube.com/playlist?list=PL1", "privacyStatus": "unlisted", "itemCount": 3.0} {
			if got[0][k] != want {
				t.Errorf("%s = %v, want %v", k, got[0][k], want)
			}
		}
	})

	t.Run("--json は、再生リストがなくても [] を出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, _ = withPlaylists(d)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"--json", "playlist", "ls"})

		// Assert
		if code != 0 || strings.TrimSpace(out.String()) != "[]" {
			t.Errorf("code=%d out=%q", code, out.String())
		}
	})

	t.Run("人向けの出力は、名前・URL・公開範囲・動画の数を1行に出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d, _ = withPlaylists(d, cliPlaylist("PL1", "Go 入門"))
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(ctx, d, []string{"playlist", "ls"})

		// Assert
		want := "Go 入門\thttps://www.youtube.com/playlist?list=PL1\tunlisted\t3\n"
		if code != 0 || out.String() != want {
			t.Errorf("code=%d out=%q want=%q", code, out.String(), want)
		}
	})
}

func TestPlaylistCreateCommand(t *testing.T) {
	ctx := context.Background()

	t.Run("説明を付けて作り、created・名前・URL を出す", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, yt := withPlaylists(d)
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"playlist", "create", "Go 入門", "--description", "Go を学ぶ"})

		// Assert
		want := "created\tGo 入門\thttps://www.youtube.com/playlist?list=PLnew\n"
		if code != 0 || out.String() != want {
			t.Fatalf("code=%d out=%q want=%q stderr=%q", code, out.String(), want, errb.String())
		}
		if len(yt.Created) != 1 || yt.Created[0].Title != "Go 入門" || yt.Created[0].Description != dv.AddPlaylistMarker("Go を学ぶ") {
			t.Errorf("created=%v", yt.Created)
		}
	})

	t.Run("--json は id・title・url・privacyStatus・warnings を出す", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, _ = withPlaylists(d)
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"--json", "playlist", "create", "Go 入門"})

		// Assert
		var got map[string]any
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || code != 0 {
			t.Fatalf("code=%d err=%v out=%q stderr=%q", code, err, out.String(), errb.String())
		}
		for k, want := range map[string]any{"id": "PLnew", "title": "Go 入門", "url": "https://www.youtube.com/playlist?list=PLnew", "privacyStatus": "unlisted"} {
			if got[k] != want {
				t.Errorf("%s = %v, want %v", k, got[k], want)
			}
		}
		if w, ok := got["warnings"].([]any); !ok || len(w) != 0 {
			t.Errorf("warnings = %v", got["warnings"])
		}
	})

	t.Run("同じ名前の CLI 再生リストがあれば 5", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, yt := withPlaylists(d, cliPlaylist("PL1", "Go 入門"))
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"playlist", "create", "Go 入門"})

		// Assert
		if code != 5 || len(yt.Created) != 0 || out.Len() != 0 {
			t.Errorf("code=%d created=%v out=%q", code, yt.Created, out.String())
		}
	})

	t.Run("名前を指定しなければ 2", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, _ = withPlaylists(d)
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"playlist", "create"})

		// Assert
		if code != 2 {
			t.Errorf("code=%d", code)
		}
	})
}

func TestPlaylistAddCommand(t *testing.T) {
	ctx := context.Background()

	t.Run("動画を入れ、added・動画 ID・再生リストの名前と URL を出す", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, yt := withPlaylists(d, cliPlaylist("PL1", "Go 入門"))
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"playlist", "add", "v1", "Go 入門"})

		// Assert
		want := "added\tv1\tGo 入門\thttps://www.youtube.com/playlist?list=PL1\n"
		if code != 0 || out.String() != want || len(yt.AddedToPlaylist) != 1 {
			t.Errorf("code=%d out=%q want=%q stderr=%q", code, out.String(), want, errb.String())
		}
	})

	t.Run("すでに入っていれば exists と出して 0", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, yt := withPlaylists(d, cliPlaylist("PL1", "Go 入門"))
		yt.PlaylistVideoIDs["PL1"] = []string{"v1"}
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"playlist", "add", "v1", "PL1"})

		// Assert
		want := "exists\tv1\tGo 入門\thttps://www.youtube.com/playlist?list=PL1\n"
		if code != 0 || out.String() != want || len(yt.AddedToPlaylist) != 0 {
			t.Errorf("code=%d out=%q want=%q", code, out.String(), want)
		}
	})

	t.Run("--json は videoId・playlistId・playlistTitle・playlistUrl・added を出す", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, _ = withPlaylists(d, cliPlaylist("PL1", "Go 入門"))
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"--json", "playlist", "add", "v1", "PL1"})

		// Assert
		var got map[string]any
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || code != 0 {
			t.Fatalf("code=%d err=%v out=%q stderr=%q", code, err, out.String(), errb.String())
		}
		for k, want := range map[string]any{"videoId": "v1", "playlistId": "PL1", "playlistTitle": "Go 入門", "playlistUrl": "https://www.youtube.com/playlist?list=PL1", "added": true} {
			if got[k] != want {
				t.Errorf("%s = %v, want %v", k, got[k], want)
			}
		}
	})

	t.Run("公開に変えられた再生リストには入れずに 7", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		public := cliPlaylist("PL1", "Go 入門")
		public.PrivacyStatus = "public"
		d, yt := withPlaylists(d, public)
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"playlist", "add", "v1", "PL1"})

		// Assert
		if code != 7 || len(yt.AddedToPlaylist) != 0 {
			t.Errorf("code=%d added=%v stderr=%q", code, yt.AddedToPlaylist, errb.String())
		}
	})

	t.Run("CLI の動画でなければ 4", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, yt := withPlaylists(d, cliPlaylist("PL1", "Go 入門"))
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"playlist", "add", "x1", "PL1"})

		// Assert
		if code != 4 || len(yt.AddedToPlaylist) != 0 {
			t.Errorf("code=%d stderr=%q", code, errb.String())
		}
	})

	t.Run("同じ名前の CLI 再生リストが2つあれば 6 で、候補の URL を出す", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, _ = withPlaylists(d, cliPlaylist("PL1", "同じ"), cliPlaylist("PL2", "同じ"))
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"playlist", "add", "v1", "同じ"})

		// Assert
		if code != 6 || !strings.Contains(errb.String(), "list=PL1") || !strings.Contains(errb.String(), "list=PL2") {
			t.Errorf("code=%d stderr=%q", code, errb.String())
		}
	})

	t.Run("引数が足りなければ 2", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d, _ = withPlaylists(d)
		initAndClear(t, d, out, errb)
		writePolicy(t, dir, "")

		// Act
		code := Execute(ctx, d, []string{"playlist", "add", "v1"})

		// Assert
		if code != 2 {
			t.Errorf("code=%d", code)
		}
	})
}
