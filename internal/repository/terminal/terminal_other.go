//go:build !windows

package terminal

import "os"

func isCygwinPTY(*os.File) bool { return false }

// EnableVT は、Windows 以外ではいつも true を返す（仮想端末の処理は要らない）。
func EnableVT(*os.File) bool { return true }
