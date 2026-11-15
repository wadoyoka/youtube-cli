package youtubeapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestIsRetryable(t *testing.T) {
	rate := &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "userRateLimitExceeded"}}}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"429", &googleapi.Error{Code: 429}, true},
		{"500番台", &googleapi.Error{Code: 502}, true},
		{"403 のレート制限", rate, true},
		{"403 のその他", &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "forbidden"}}}, false},
		{"404", &googleapi.Error{Code: 404}, false},
		{"タイムアウト", timeoutErr{}, true},
		{"UnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"接続のリセット", &url.Error{Op: "Post", URL: "x", Err: &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}}, true},
		{"切れた接続への書き込み", &url.Error{Op: "Post", URL: "x", Err: &net.OpError{Op: "write", Net: "tcp", Err: os.NewSyscallError("write", syscall.EPIPE)}}, true},
		{"応答の前に接続が閉じられた", &url.Error{Op: "Post", URL: "x", Err: io.EOF}, true},
		{"中断", &url.Error{Op: "Post", URL: "x", Err: context.Canceled}, false},
		{"その他", errors.New("x"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// Act
			got := isRetryable(tc.err)
			// Assert
			if got != tc.want {
				t.Errorf("isRetryable = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMayHaveCreated(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"500番台は作られたかもしれない", &googleapi.Error{Code: 503}, true},
		{"応答の前に接続が閉じられたら作られたかもしれない", &url.Error{Op: "Post", URL: "x", Err: io.EOF}, true},
		{"中断も作られたかもしれない", &url.Error{Op: "Post", URL: "x", Err: context.Canceled}, true},
		{"分からない失敗も作られたかもしれない", errors.New("x"), true},
		{"429 は作られていない", &googleapi.Error{Code: 429}, false},
		{"403 のクォータ切れは作られていない", &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "quotaExceeded"}}}, false},
		{"404 は作られていない", &googleapi.Error{Code: 404}, false},
		{"トークンを更新できなければ送っていない", &url.Error{Op: "Post", URL: "x", Err: &oauth2.RetrieveError{ErrorCode: "invalid_grant"}}, false},
		{"鍵を読めないなど domain のエラーなら送っていない", &url.Error{Op: "Post", URL: "x", Err: domain.Errorf(domain.KindSetup, "鍵がありません")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// Act
			got := mayHaveCreated(tc.err)
			// Assert
			if got != tc.want {
				t.Errorf("mayHaveCreated = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRetry(t *testing.T) {
	t.Run("最大回数で止まり、待ち時間は倍々になる", func(t *testing.T) {
		// Arrange
		var slept []time.Duration
		c := &Client{attempts: 5, sleep: func(d time.Duration) { slept = append(slept, d) }}
		calls := 0
		// Act
		err := c.retry(context.Background(), isRetryable, func() error { calls++; return &googleapi.Error{Code: 503} })
		// Assert
		if err == nil || calls != 5 || len(slept) != 4 || slept[3] != 8*time.Second {
			t.Errorf("err = %v, calls = %d, slept = %v", err, calls, slept)
		}
	})

	t.Run("やり直せない失敗はすぐ返す", func(t *testing.T) {
		// Arrange
		c := &Client{attempts: 5, sleep: func(time.Duration) { t.Error("sleep が呼ばれた") }}
		calls := 0
		// Act
		_ = c.retry(context.Background(), isRetryable, func() error { calls++; return &googleapi.Error{Code: 404} })
		// Assert
		if calls != 1 {
			t.Errorf("calls = %d", calls)
		}
	})
}
