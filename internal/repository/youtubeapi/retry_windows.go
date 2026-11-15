//go:build windows

package youtubeapi

import "syscall"

// errorNetnameDeleted は ERROR_NETNAME_DELETED（64）。接続が切れたとき、wsarecv: The specified network name is no longer available. で返ることがある。
const errorNetnameDeleted = syscall.Errno(64)

// connErrnos は、接続が切れたことを表す OS のエラー番号の一覧。Windows では WSA のエラー番号も使う。
var connErrnos = []syscall.Errno{syscall.ECONNRESET, syscall.EPIPE, syscall.WSAECONNRESET, syscall.WSAECONNABORTED, errorNetnameDeleted}
