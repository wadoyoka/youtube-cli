package configdir

import (
	"path/filepath"
	"testing"
)

func TestResolve(t *testing.T) {
	cases := []struct {
		name string
		vars map[string]string
		want string
	}{
		// 期待値は filepath.Join で作る（Windows では \ 区切りになる）。
		{"環境変数なし", nil, filepath.Join("/h", ".config", "youtube-cli")},
		{"XDG_CONFIG_HOME", map[string]string{"XDG_CONFIG_HOME": "/x"}, filepath.Join("/x", "youtube-cli")},
		{"YOUTUBE_CLI_CONFIG_DIR", map[string]string{"YOUTUBE_CLI_CONFIG_DIR": "/c"}, "/c"},
		{"両方あれば YOUTUBE_CLI_CONFIG_DIR", map[string]string{"YOUTUBE_CLI_CONFIG_DIR": "/c", "XDG_CONFIG_HOME": "/x"}, "/c"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Arrange
			getenv := func(k string) string { return c.vars[k] }

			// Act
			got := Resolve(getenv, "/h")

			// Assert
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestResolveWindows(t *testing.T) {
	const local = `C:\Users\u\AppData\Local`
	const localJa = `C:\Users\山田 太郎\AppData\Local`
	cases := []struct {
		name string
		vars map[string]string
		want string
	}{
		{"LOCALAPPDATA の下", map[string]string{"LOCALAPPDATA": local}, filepath.Join(local, "youtube-cli")},
		{"XDG_CONFIG_HOME があっても LOCALAPPDATA の下", map[string]string{"LOCALAPPDATA": local, "XDG_CONFIG_HOME": "/x"}, filepath.Join(local, "youtube-cli")},
		{"YOUTUBE_CLI_CONFIG_DIR があればそれ", map[string]string{"LOCALAPPDATA": local, "YOUTUBE_CLI_CONFIG_DIR": `D:\cfg`}, `D:\cfg`},
		{"空白と日本語を含む LOCALAPPDATA", map[string]string{"LOCALAPPDATA": localJa}, filepath.Join(localJa, "youtube-cli")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Arrange
			getenv := func(k string) string { return c.vars[k] }

			// Act
			got := ResolveWindows(getenv)

			// Assert
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
