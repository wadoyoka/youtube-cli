package youtubeapi

import (
	"context"
	"encoding/json"
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

func TestListPlaylistVideoIDs(t *testing.T) {
	t.Run("全ページをたどり、動画の ID を順に返す", func(t *testing.T) {
		// Arrange
		var queries []url.Values
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			queries = append(queries, r.URL.Query())
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("pageToken") == "" {
				_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"videoId":"a"}},{"contentDetails":{"videoId":"b"}}],"nextPageToken":"p2"}`))
				return
			}
			_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"videoId":"c"}}]}`))
		})

		// Act
		got, err := c.ListPlaylistVideoIDs(context.Background(), "UU1")

		// Assert
		if err != nil || strings.Join(got, ",") != "a,b,c" {
			t.Fatalf("got=%v err=%v", got, err)
		}
		if len(queries) != 2 || queries[0].Get("playlistId") != "UU1" || queries[0].Get("maxResults") != "50" || queries[0].Get("part") != "contentDetails" || queries[1].Get("pageToken") != "p2" {
			t.Errorf("queries=%v", queries)
		}
	})

	t.Run("403 のクォータ切れは KindQuota", func(t *testing.T) {
		// Arrange
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":403,"message":"quota","errors":[{"reason":"quotaExceeded","message":"quota"}]}}`))
		})

		// Act
		_, err := c.ListPlaylistVideoIDs(context.Background(), "UU1")

		// Assert
		if domain.KindOf(err) != domain.KindQuota {
			t.Errorf("err=%v", err)
		}
	})
}

func TestGetVideos(t *testing.T) {
	t.Run("50 件ずつ引き、動画の項目を変換して返す", func(t *testing.T) {
		// Arrange
		var batches []int
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			ids := r.URL.Query()["id"]
			batches = append(batches, len(ids))
			if strings.Join(r.URL.Query()["part"], ",") != "snippet,status" {
				t.Errorf("part = %v", r.URL.Query()["part"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{
				"id":      ids[0],
				"snippet": map[string]any{"title": "題", "description": "説明", "tags": []string{"t1"}, "publishedAt": "2026-01-02T03:04:05Z"},
				"status":  map[string]any{"privacyStatus": "unlisted", "uploadStatus": "processed"},
			}}})
		})
		ids := make([]string, 120)
		for i := range ids {
			ids[i] = "id" + strings.Repeat("x", i%3) + string(rune('a'+i%26)) + string(rune('a'+i/26))
		}

		// Act
		got, err := c.GetVideos(context.Background(), ids)

		// Assert
		if err != nil || len(got) != 3 {
			t.Fatalf("got=%v err=%v", got, err)
		}
		if len(batches) != 3 || batches[0] != 50 || batches[1] != 50 || batches[2] != 20 {
			t.Errorf("batches=%v", batches)
		}
		v := got[0]
		if v.ID != ids[0] || v.Title != "題" || v.Description != "説明" || len(v.Tags) != 1 || v.PrivacyStatus != "unlisted" || v.UploadStatus != "processed" || v.PublishedAt.Year() != 2026 {
			t.Errorf("v=%+v", v)
		}
	})

	t.Run("ID が空なら API を呼ばない", func(t *testing.T) {
		// Arrange
		var calls atomic.Int32
		c := newTestClient(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })

		// Act
		got, err := c.GetVideos(context.Background(), nil)

		// Assert
		if err != nil || len(got) != 0 || calls.Load() != 0 {
			t.Errorf("got=%v err=%v calls=%d", got, err, calls.Load())
		}
	})
}
