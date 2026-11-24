// Package configdir は、設定ディレクトリの場所の決め方を持つ。
package configdir

import "path/filepath"

// Resolve は、設定ディレクトリを決める。
// YOUTUBE_CLI_CONFIG_DIR、$XDG_CONFIG_HOME/youtube-cli、~/.config/youtube-cli の順。
func Resolve(getenv func(string) string, home string) string {
	if d := getenv("YOUTUBE_CLI_CONFIG_DIR"); d != "" {
		return d
	}
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "youtube-cli")
	}
	return filepath.Join(home, ".config", "youtube-cli")
}

// ResolveWindows は、Windows の設定ディレクトリを決める。
// YOUTUBE_CLI_CONFIG_DIR、%LOCALAPPDATA%\youtube-cli の順。XDG_CONFIG_HOME は見ない。
func ResolveWindows(getenv func(string) string) string {
	if d := getenv("YOUTUBE_CLI_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(getenv("LOCALAPPDATA"), "youtube-cli")
}
