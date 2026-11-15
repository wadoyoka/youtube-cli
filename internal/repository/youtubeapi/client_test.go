package youtubeapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	svc, err := youtube.NewService(context.Background(), option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(svc)
	c.sleep = func(time.Duration) {}
	return c
}

func TestMyChannel(t *testing.T) {
	t.Run("mine=true で引き、チャンネルの ID・名前・アップロード再生リストを返す", func(t *testing.T) {
		// Arrange
		var gotQuery atomic.Value
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotQuery.Store(r.URL.Query())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":"UC1","snippet":{"title":"私のチャンネル"},"contentDetails":{"relatedPlaylists":{"uploads":"UU1"}}}]}`))
		})

		// Act
		got, err := c.MyChannel(context.Background())

		// Assert
		want := domain.Channel{ID: "UC1", Title: "私のチャンネル", UploadsPlaylistID: "UU1"}
		if err != nil || got != want {
			t.Fatalf("got %+v, %v; want %+v", got, err, want)
		}
		q := gotQuery.Load().(url.Values)
		if q["mine"][0] != "true" || strings.Join(q["part"], ",") != "snippet,contentDetails" {
			t.Errorf("query = %v", q)
		}
	})

	t.Run("チャンネルがなければ KindSetup", func(t *testing.T) {
		// Arrange
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[]}`))
		})

		// Act
		_, err := c.MyChannel(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("403 のクォータ切れは KindQuota で、やり直さない", func(t *testing.T) {
		// Arrange
		var calls atomic.Int32
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":403,"message":"quota","errors":[{"reason":"quotaExceeded","message":"quota"}]}}`))
		})

		// Act
		_, err := c.MyChannel(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindQuota || calls.Load() != 1 {
			t.Errorf("err=%v calls=%d", err, calls.Load())
		}
	})

	t.Run("503 は待ってやり直す", func(t *testing.T) {
		// Arrange
		var calls atomic.Int32
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"code":503,"message":"busy"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"UC1","snippet":{"title":"t"}}]}`))
		})

		// Act
		got, err := c.MyChannel(context.Background())

		// Assert
		if err != nil || got.ID != "UC1" || calls.Load() != 2 {
			t.Errorf("got=%+v err=%v calls=%d", got, err, calls.Load())
		}
	})
}
