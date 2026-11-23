//go:build windows

package youtubeapi

import (
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
)

func TestIsRetryableWindows(t *testing.T) {
	cases := []struct {
		name  string
		errno syscall.Errno
		want  bool
	}{
		{"接続のリセット（WSAECONNRESET）", syscall.WSAECONNRESET, true},
		{"接続の中止（WSAECONNABORTED）", syscall.WSAECONNABORTED, true},
		{"ネットワーク名の消失（ERROR_NETNAME_DELETED）", syscall.Errno(64), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			err := &url.Error{Op: "Post", URL: "x", Err: &net.OpError{Op: "wsarecv", Net: "tcp", Err: os.NewSyscallError("wsarecv", tc.errno)}}
			// Act
			got := isRetryable(err)
			// Assert
			if got != tc.want {
				t.Errorf("isRetryable = %v, want %v", got, tc.want)
			}
		})
	}
}
