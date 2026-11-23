package youtubeapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

// isRetryable は、やり直せる失敗かどうかを返す。
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if isRateLimited(err) {
		return true
	}
	if gerr, ok := errors.AsType[*googleapi.Error](err); ok {
		return gerr.Code >= 500
	}
	return isNetworkError(err)
}

// isRateLimited は、混み合いで断られた失敗（429、403 の rateLimitExceeded と userRateLimitExceeded）かどうかを返す。
// 断られた要求は処理されていないので、何も作られていない。
func isRateLimited(err error) bool {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		return false
	}
	switch gerr.Code {
	case http.StatusTooManyRequests:
		return true
	case http.StatusForbidden:
		for _, e := range gerr.Errors {
			if e.Reason == "rateLimitExceeded" || e.Reason == "userRateLimitExceeded" {
				return true
			}
		}
	}
	return false
}

// mayHaveCreated は、何かを作る要求（再生リストを作る、再生リストに入れる）が err で失敗したとき、YouTube で作られたかもしれないかを返す。
// 作られていないと言い切れるのは、YouTube が 4xx で断ったとき（混み合い、クォータ切れ、見つからないなど）と、
// トークンを更新できないなどで要求を送れなかったとき（oauth2.RetrieveError、domain.Error）だけ。
// 5xx、通信の失敗、そのほかの分からない失敗は、作られたかもしれない側に倒す。
func mayHaveCreated(err error) bool {
	if gerr, ok := errors.AsType[*googleapi.Error](err); ok {
		return gerr.Code < 400 || gerr.Code >= 500
	}
	if _, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
		return false
	}
	if _, ok := errors.AsType[*domain.Error](err); ok {
		return false
	}
	return true
}

// isNetworkError は、やり直してよい通信の失敗（時間切れ、接続のリセット、途中で切れた応答など）かどうかを返す。
func isNetworkError(err error) bool {
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return true
	}
	// 要求を送ったあと、応答の前に接続が閉じられたとき、net/http は io.EOF を url.Error に包んで返す。
	var uerr *url.Error
	if errors.As(err, &uerr) && errors.Is(uerr.Err, io.EOF) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	for _, errno := range connErrnos {
		if errors.Is(err, errno) {
			return true
		}
	}
	return false
}

// retry は、fn が retryable の失敗を返したとき、1s、2s、4s、8s と待って、最大 attempts 回まで呼ぶ。
func (c *Client) retry(ctx context.Context, retryable func(error) bool, fn func() error) error {
	var err error
	for attempt := 1; attempt <= c.attempts; attempt++ {
		err = fn()
		if err == nil || !retryable(err) || attempt == c.attempts {
			return err
		}
		if ctx.Err() != nil {
			return err
		}
		c.sleep(backoff(attempt))
	}
	return err
}

// backoff は、attempt 回目の失敗のあとに待つ時間（1s、2s、4s、8s）を返す。
func backoff(attempt int) time.Duration {
	return time.Second << (attempt - 1)
}
