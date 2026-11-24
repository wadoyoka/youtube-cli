package config

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

func TestLedgerFile(t *testing.T) {
	t.Run("足した動画と再生リストを読み戻せ、権限は0600", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		lf := NewLedgerFile(dir)
		v := usecase.LedgerVideo{ID: "v1", Title: "動画", Size: 123, UploadedAt: time.Date(2026, 11, 16, 1, 2, 3, 0, time.UTC)}
		p := usecase.LedgerPlaylist{ID: "p1", Title: "リスト"}

		// Act
		err1 := lf.AddVideo(v)
		err2 := lf.AddPlaylist(p)
		vs, err3 := lf.Videos()
		ps, err4 := lf.Playlists()
		info, statErr := os.Stat(dir + "/ledger.json")

		// Assert
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || statErr != nil {
			t.Fatalf("errors: %v %v %v %v %v", err1, err2, err3, err4, statErr)
		}
		if len(vs) != 1 || !vs[0].UploadedAt.Equal(v.UploadedAt) || vs[0].ID != v.ID || vs[0].Title != v.Title || vs[0].Size != v.Size {
			t.Errorf("videos = %+v", vs)
		}
		if len(ps) != 1 || ps[0] != p {
			t.Errorf("playlists = %+v", ps)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Errorf("perm = %v", info.Mode().Perm())
		}
	})

	t.Run("ファイルがなければ空", func(t *testing.T) {
		// Arrange
		lf := NewLedgerFile(t.TempDir())

		// Act
		vs, err1 := lf.Videos()
		ps, err2 := lf.Playlists()

		// Assert
		if err1 != nil || err2 != nil || len(vs) != 0 || len(ps) != 0 {
			t.Errorf("vs=%v ps=%v err=%v %v", vs, ps, err1, err2)
		}
	})

	t.Run("壊れていればパスを含むエラー", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		if err := os.WriteFile(dir+"/ledger.json", []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		lf := NewLedgerFile(dir)

		// Act
		_, err := lf.Videos()
		addErr := lf.AddVideo(usecase.LedgerVideo{ID: "v"})

		// Assert
		if err == nil || !strings.Contains(err.Error(), "ledger.json") {
			t.Errorf("err = %v", err)
		}
		if addErr == nil {
			t.Error("壊れたファイルへの追加はエラーのはず")
		}
	})

	t.Run("Clear で空になる", func(t *testing.T) {
		// Arrange
		lf := NewLedgerFile(t.TempDir())
		if err := lf.AddVideo(usecase.LedgerVideo{ID: "v1"}); err != nil {
			t.Fatal(err)
		}
		if err := lf.AddPlaylist(usecase.LedgerPlaylist{ID: "p1"}); err != nil {
			t.Fatal(err)
		}

		// Act
		err := lf.Clear()
		vs, _ := lf.Videos()
		ps, _ := lf.Playlists()

		// Assert
		if err != nil || len(vs) != 0 || len(ps) != 0 {
			t.Errorf("err=%v vs=%v ps=%v", err, vs, ps)
		}
	})

	t.Run("同じ ID を2回足しても1件", func(t *testing.T) {
		// Arrange
		lf := NewLedgerFile(t.TempDir())

		// Act
		_ = lf.AddVideo(usecase.LedgerVideo{ID: "v1", Title: "a"})
		_ = lf.AddVideo(usecase.LedgerVideo{ID: "v1", Title: "b"})
		_ = lf.AddPlaylist(usecase.LedgerPlaylist{ID: "p1"})
		_ = lf.AddPlaylist(usecase.LedgerPlaylist{ID: "p1"})
		vs, _ := lf.Videos()
		ps, _ := lf.Playlists()

		// Assert
		if len(vs) != 1 || len(ps) != 1 {
			t.Errorf("vs=%v ps=%v", vs, ps)
		}
	})
}
