package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestConfigDir(t *testing.T) {
	t.Run("初回は作成して0700、2回目は作成しない", func(t *testing.T) {
		// Arrange
		path := filepath.Join(t.TempDir(), "cfg")
		d := NewDir(path)

		// Act
		first, err1 := d.Ensure()
		info, statErr := os.Stat(path)
		second, err2 := d.Ensure()

		// Assert
		if err1 != nil || err2 != nil || statErr != nil {
			t.Fatalf("errors: %v %v %v", err1, err2, statErr)
		}
		if !first || second {
			t.Errorf("created: first=%v second=%v", first, second)
		}
		t.Run("権限は 0700", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows ではファイルの権限の数字が効かない")
			}
			if info.Mode().Perm() != 0o700 {
				t.Errorf("perm = %v", info.Mode().Perm())
			}
		})
		if d.Path() != path {
			t.Errorf("Path = %q", d.Path())
		}
	})
}

func TestDefaultDir(t *testing.T) {
	t.Run("YOUTUBE_CLI_CONFIG_DIR があれば HOME がなくても決まる", func(t *testing.T) {
		// Arrange
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("YOUTUBE_CLI_CONFIG_DIR", filepath.Join("tmp", "youtube-cli-test"))

		// Act
		got, err := defaultDir("linux")

		// Assert
		if err != nil || got != filepath.Join("tmp", "youtube-cli-test") {
			t.Errorf("got = %q, err = %v", got, err)
		}
	})

	t.Run("XDG_CONFIG_HOME があれば HOME がなくても決まる", func(t *testing.T) {
		// Arrange
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("XDG_CONFIG_HOME", filepath.Join("tmp", "xdg"))
		t.Setenv("YOUTUBE_CLI_CONFIG_DIR", "")

		// Act
		got, err := defaultDir("linux")

		// Assert
		if err != nil || got != filepath.Join("tmp", "xdg", "youtube-cli") {
			t.Errorf("got = %q, err = %v", got, err)
		}
	})

	t.Run("どれもなく HOME もなければエラー", func(t *testing.T) {
		// Arrange
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("YOUTUBE_CLI_CONFIG_DIR", "")

		// Act
		_, err := defaultDir("linux")

		// Assert
		if err == nil {
			t.Error("err = nil")
		}
	})

	t.Run("Windows では LOCALAPPDATA の下で、XDG_CONFIG_HOME を見ない", func(t *testing.T) {
		// Arrange
		local := filepath.Join("local", "app")
		t.Setenv("LOCALAPPDATA", local)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join("tmp", "xdg"))
		t.Setenv("YOUTUBE_CLI_CONFIG_DIR", "")

		// Act
		got, err := defaultDir("windows")

		// Assert
		if err != nil || got != filepath.Join(local, "youtube-cli") {
			t.Errorf("got = %q, err = %v", got, err)
		}
	})

	t.Run("Windows で LOCALAPPDATA も YOUTUBE_CLI_CONFIG_DIR も空ならエラー", func(t *testing.T) {
		// Arrange
		t.Setenv("LOCALAPPDATA", "")
		t.Setenv("YOUTUBE_CLI_CONFIG_DIR", "")

		// Act
		_, err := defaultDir("windows")

		// Assert
		if err == nil {
			t.Error("err = nil")
		}
	})

	t.Run("Windows で YOUTUBE_CLI_CONFIG_DIR があれば LOCALAPPDATA が空でも決まる", func(t *testing.T) {
		// Arrange
		t.Setenv("LOCALAPPDATA", "")
		t.Setenv("YOUTUBE_CLI_CONFIG_DIR", filepath.Join("tmp", "cfg"))

		// Act
		got, err := defaultDir("windows")

		// Assert
		if err != nil || got != filepath.Join("tmp", "cfg") {
			t.Errorf("got = %q, err = %v", got, err)
		}
	})
}
