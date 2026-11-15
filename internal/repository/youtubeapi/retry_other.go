//go:build !windows

package youtubeapi

import "syscall"

// connErrnos は、接続が切れたことを表す OS のエラー番号の一覧。
var connErrnos = []syscall.Errno{syscall.ECONNRESET, syscall.EPIPE}
