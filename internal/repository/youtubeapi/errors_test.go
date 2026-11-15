package youtubeapi

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

func TestMapError(t *testing.T) {
	t.Run("403 のクォータ切れは KindQuota", func(t *testing.T) {
		for _, reason := range []string{"quotaExceeded", "uploadLimitExceeded", "dailyLimitExceeded"} {
			// Arrange
			err := &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: reason}}}

			// Act
			got := mapError(err)

			// Assert
			if domain.KindOf(got) != domain.KindQuota {
				t.Errorf("%s: got = %v (%v)", reason, got, domain.KindOf(got))
			}
		}
	})

	t.Run("403 のその他は KindUnexpected", func(t *testing.T) {
		// Arrange
		err := &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "forbidden"}}}

		// Act
		got := mapError(err)

		// Assert
		if domain.KindOf(got) != domain.KindUnexpected {
			t.Errorf("got = %v", got)
		}
	})

	t.Run("リフレッシュ失敗は KindSetup", func(t *testing.T) {
		// Arrange
		err := &url.Error{Op: "Get", URL: "x", Err: &oauth2.RetrieveError{Response: &http.Response{StatusCode: 400}, ErrorCode: "invalid_grant"}}

		// Act
		got := mapError(err)

		// Assert
		if domain.KindOf(got) != domain.KindSetup || !strings.Contains(got.Error(), "youtube-cli login") {
			t.Errorf("got = %v", got)
		}
	})

	t.Run("404 は KindNotFound", func(t *testing.T) {
		// Arrange
		err := &googleapi.Error{Code: 404}

		// Act
		got := mapError(err)

		// Assert
		if domain.KindOf(got) != domain.KindNotFound {
			t.Errorf("got = %v", got)
		}
	})

	t.Run("それ以外は KindUnexpected", func(t *testing.T) {
		// Arrange
		// Act
		got := mapError(io.ErrClosedPipe)

		// Assert
		if domain.KindOf(got) != domain.KindUnexpected {
			t.Errorf("got = %v", got)
		}
	})

	t.Run("nil は nil", func(t *testing.T) {
		// Arrange
		// Act
		got := mapError(nil)

		// Assert
		if got != nil {
			t.Errorf("got = %v", got)
		}
	})

	t.Run("エラーには URL の問い合わせ部分を残さない", func(t *testing.T) {
		// Arrange
		err := &url.Error{Op: "Put", URL: "https://example.com/upload/youtube/v3/videos?uploadType=resumable&upload_id=SESSION", Err: io.ErrUnexpectedEOF}
		cause := domain.Errorf(domain.KindConflict, "a.mp4 を送れません").Wrap(&url.Error{Op: "Post", URL: "https://example.com/videos?upload_id=SESSION2", Err: io.EOF})

		// Act
		got1 := mapError(err)
		got2 := mapError(cause)

		// Assert
		if s := got1.Error(); strings.Contains(s, "SESSION") || !strings.Contains(s, `Put "https://example.com/upload/youtube/v3/videos"`) {
			t.Errorf("got1 = %q", s)
		}
		if s := got2.Error(); strings.Contains(s, "SESSION2") || domain.KindOf(got2) != domain.KindConflict {
			t.Errorf("got2 = %q", s)
		}
	})
}
