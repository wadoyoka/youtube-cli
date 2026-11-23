// Package browser は、既定のブラウザで URL を開く。
package browser

import (
	"os/exec"
	"runtime"
)

// Open は、url を既定のブラウザで開く。
// Windows は rundll32、darwin は open、それ以外は xdg-open。
func Open(url string) error {
	name, args := command(runtime.GOOS, url)
	cmd := exec.Command(name, args...) //nolint:gosec // 固定のコマンド名で、url は自分で組み立てた値
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// command は、goos で URL を開くコマンドの名前と引数を返す。
// cmd /c start は、url の & を区切りとして解釈するため使わない。
func command(goos, url string) (name string, args []string) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		return "open", []string{url}
	default:
		return "xdg-open", []string{url}
	}
}
