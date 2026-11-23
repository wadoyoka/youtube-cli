package config

import (
	"os"
	"runtime"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

func TestStateFile(t *testing.T) {
	t.Run("保存して読み直すと同じチャンネルで、権限は0600", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		sf := NewStateFile(dir)
		want := domain.Channel{ID: "UC1", Title: "テスト", UploadsPlaylistID: "UU1"}

		// Act
		err := sf.SetChannel(want)
		got, found, err2 := sf.Channel()
		info, statErr := os.Stat(dir + "/state.json")

		// Assert
		if err != nil || err2 != nil || statErr != nil {
			t.Fatalf("errors: %v %v %v", err, err2, statErr)
		}
		if !found || got != want {
			t.Errorf("got = %+v found=%v", got, found)
		}
		t.Run("権限は 0600", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows ではファイルの権限の数字が効かない")
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("perm = %v", info.Mode().Perm())
			}
		})
	})

	t.Run("ファイルがなければ found=false", func(t *testing.T) {
		// Arrange
		sf := NewStateFile(t.TempDir())

		// Act
		got, found, err := sf.Channel()

		// Assert
		if err != nil || found || got != (domain.Channel{}) {
			t.Errorf("got=%+v found=%v err=%v", got, found, err)
		}
	})

	t.Run("state.json のキーは channelId・channelTitle・uploadsPlaylistId", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		sf := NewStateFile(dir)

		// Act
		err := sf.SetChannel(domain.Channel{ID: "UC1", Title: "t", UploadsPlaylistID: "UU1"})
		raw, readErr := os.ReadFile(dir + "/state.json") //nolint:gosec // テストの一時ディレクトリ

		// Assert
		if err != nil || readErr != nil {
			t.Fatalf("errors: %v %v", err, readErr)
		}
		if want := `{"channelId":"UC1","channelTitle":"t","uploadsPlaylistId":"UU1"}`; string(raw) != want {
			t.Errorf("raw = %s", raw)
		}
	})

	t.Run("壊れた JSON はエラー", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		if err := os.WriteFile(dir+"/state.json", []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		_, _, err := NewStateFile(dir).Channel()

		// Assert
		if err == nil {
			t.Error("エラーになるはず")
		}
	})
}
