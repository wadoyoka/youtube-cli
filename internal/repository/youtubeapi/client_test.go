package youtubeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/domain/video"
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
			if q := r.URL.Query(); strings.Join(q["part"], ",") != "snippet,status" || q.Has("maxResults") || q.Has("pageToken") {
				t.Errorf("ID で引くときは maxResults と pageToken を送らない: query = %v", q)
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

// sentRequest は、偽の YouTube が受け取った要求。
type sentRequest struct {
	header http.Header
	query  url.Values
	body   []byte
}

// uploadServer は、InsertVideo を試すための偽の YouTube。
// 動画を上げる最初の要求（/upload/youtube/v3/videos）には starts の順に、
// 再開可能アップロードの続き（/upload/session）には chunks の順に応える。尽きたら最後のものを使う。
type uploadServer struct {
	mu      sync.Mutex
	starts  []http.HandlerFunc
	chunks  []http.HandlerFunc
	started []sentRequest
	chunked []sentRequest
}

func (s *uploadServer) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	req := sentRequest{header: r.Header.Clone(), query: r.URL.Query(), body: body}
	s.mu.Lock()
	var respond http.HandlerFunc
	if strings.HasPrefix(r.URL.Path, "/upload/session") {
		s.chunked = append(s.chunked, req)
		respond = s.chunks[min(len(s.chunked), len(s.chunks))-1]
	} else {
		s.started = append(s.started, req)
		respond = s.starts[min(len(s.started), len(s.starts))-1]
	}
	s.mu.Unlock()
	respond(w, r)
}

// startReqs は、受け取った最初の要求（動画を上げる要求）を返す。
func (s *uploadServer) startReqs() []sentRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.started)
}

// chunkReqs は、受け取った再開可能アップロードの続きの要求を返す。
func (s *uploadServer) chunkReqs() []sentRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.chunked)
}

// created は、ID が id の動画を作った応答を返す。
func created(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      id,
			"snippet": map[string]any{"title": "題", "tags": []string{"go", "youtube-cli"}},
			"status":  map[string]any{"privacyStatus": "unlisted", "uploadStatus": "uploaded"},
		})
	}
}

// refused は、code と reason で断る応答を返す。
func refused(code int, reason string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = fmt.Fprintf(w, `{"error":{"code":%d,"message":"x","errors":[{"reason":%q,"message":"x"}]}}`, code, reason)
	}
}

// sessionStarted は、再開可能アップロードのセッションを作った応答を返す。続きの送り先には、知られると困る upload_id を入れる。
func sessionStarted(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Location", "http://"+r.Host+"/upload/session?upload_id=SECRET")
	w.WriteHeader(http.StatusOK)
}

// chunkAccepted は、再開可能アップロードの途中の塊を受け取った応答（308 の代わり）を返す。
func chunkAccepted(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("X-Http-Status-Code-Override", "308")
	w.WriteHeader(http.StatusOK)
}

// hangUp は、要求を読み終えたあと、応えずに接続を切る。
func hangUp(w http.ResponseWriter, _ *http.Request) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err == nil {
		_ = conn.Close()
	}
}

// metadataOf は、最初の要求で送った動画の項目（JSON）を返す。multipart なら最初の部分を読む。
func metadataOf(t *testing.T, r sentRequest) map[string]any {
	t.Helper()
	data := r.body
	mt, params, err := mime.ParseMediaType(r.header.Get("Content-Type"))
	if err == nil && strings.HasPrefix(mt, "multipart/") {
		part, err := multipart.NewReader(bytes.NewReader(r.body), params["boundary"]).NextPart()
		if err != nil {
			t.Fatal(err)
		}
		if data, err = io.ReadAll(part); err != nil {
			t.Fatal(err)
		}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json: %v: %q", err, data)
	}
	return m
}

func newUploadClient(t *testing.T, s *uploadServer) (*Client, *[]time.Duration) {
	t.Helper()
	c := newTestClient(t, s.handle)
	slept := &[]time.Duration{}
	c.sleep = func(d time.Duration) { *slept = append(*slept, d) }
	return c, slept
}

// roundTripFunc は、関数で応える http.RoundTripper。本物の通信を通らないので、net/http/httptrace の見張りは呼ばれない。
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// onlyReader は、io.Seeker を隠した Reader。
type onlyReader struct{ r io.Reader }

func (o onlyReader) Read(p []byte) (int, error) { return o.r.Read(p) }

func TestInsertVideo(t *testing.T) {
	meta := video.Meta{Title: "題", Description: "説明", Tags: []string{"go"}, CategoryID: video.DefaultCategory}
	const small = "small-video"
	large := bytes.Repeat([]byte("v"), uploadChunk+1)

	t.Run("限定公開に固定し、申告を明示して、目印のタグを付けて送る", func(t *testing.T) {
		// Arrange
		s := &uploadServer{starts: []http.HandlerFunc{created("vid1")}}
		c, _ := newUploadClient(t, s)

		// Act
		got, err := c.InsertVideo(context.Background(), meta, strings.NewReader(small), int64(len(small)), nil)

		// Assert
		if err != nil || got.ID != "vid1" || got.PrivacyStatus != "unlisted" || got.UploadStatus != "uploaded" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		if len(s.startReqs()) != 1 {
			t.Fatalf("要求の数=%d", len(s.startReqs()))
		}
		req := s.startReqs()[0]
		if strings.Join(req.query["part"], ",") != "snippet,status" || !bytes.Contains(req.body, []byte(small)) {
			t.Errorf("query=%v body=%q", req.query, req.body)
		}
		md := metadataOf(t, req)
		status, _ := md["status"].(map[string]any)
		if status["privacyStatus"] != "unlisted" {
			t.Errorf("status=%v", status)
		}
		for _, k := range []string{"selfDeclaredMadeForKids", "containsSyntheticMedia"} {
			if v, ok := status[k]; !ok || v != false {
				t.Errorf("status.%s=%v（送ったか: %v）", k, v, ok)
			}
		}
		snippet, _ := md["snippet"].(map[string]any)
		tags, _ := snippet["tags"].([]any)
		if snippet["title"] != "題" || snippet["description"] != "説明" || snippet["categoryId"] != "22" || !slices.Equal(tags, []any{"go", "youtube-cli"}) {
			t.Errorf("snippet=%v", snippet)
		}
	})

	t.Run("子ども向けと合成の申告は true で送る", func(t *testing.T) {
		// Arrange
		s := &uploadServer{starts: []http.HandlerFunc{created("vid1")}}
		c, _ := newUploadClient(t, s)
		m := meta
		m.MadeForKids, m.SyntheticMedia = true, true

		// Act
		_, err := c.InsertVideo(context.Background(), m, strings.NewReader(small), int64(len(small)), nil)
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		status, _ := metadataOf(t, s.startReqs()[0])["status"].(map[string]any)
		if status["selfDeclaredMadeForKids"] != true || status["containsSyntheticMedia"] != true || status["privacyStatus"] != "unlisted" {
			t.Errorf("status=%v", status)
		}
	})

	t.Run("16 MiB を超える動画は、再開可能アップロードで 16 MiB ずつ送り、進み具合を知らせる", func(t *testing.T) {
		// Arrange
		s := &uploadServer{
			starts: []http.HandlerFunc{sessionStarted},
			chunks: []http.HandlerFunc{chunkAccepted, created("vid2")},
		}
		c, _ := newUploadClient(t, s)
		var reported []int64

		// Act
		got, err := c.InsertVideo(context.Background(), meta, bytes.NewReader(large), int64(len(large)), func(n int64) { reported = append(reported, n) })

		// Assert
		if err != nil || got.ID != "vid2" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		if len(s.startReqs()) != 1 || s.startReqs()[0].query.Get("uploadType") != "resumable" {
			t.Fatalf("最初の要求=%d %v", len(s.startReqs()), s.startReqs())
		}
		if status, _ := metadataOf(t, s.startReqs()[0])["status"].(map[string]any); status["privacyStatus"] != "unlisted" {
			t.Errorf("status=%v", status)
		}
		var ranges []string
		for _, r := range s.chunkReqs() {
			ranges = append(ranges, r.header.Get("Content-Range"))
		}
		want := []string{fmt.Sprintf("bytes 0-%d/*", uploadChunk-1), fmt.Sprintf("bytes %d-%d/%d", uploadChunk, uploadChunk, uploadChunk+1)}
		if !slices.Equal(ranges, want) {
			t.Errorf("ranges=%v want=%v", ranges, want)
		}
		if len(reported) == 0 || reported[0] != 0 || !slices.Contains(reported, int64(uploadChunk)) || reported[len(reported)-1] != int64(len(large)) {
			t.Errorf("reported=%v", reported)
		}
	})

	t.Run("送信を始める前に 429 で断られたら、待って最初から送り直し、1 つだけ作る", func(t *testing.T) {
		// Arrange
		s := &uploadServer{starts: []http.HandlerFunc{refused(http.StatusTooManyRequests, "rateLimitExceeded"), created("vid3")}}
		c, slept := newUploadClient(t, s)
		var reported []int64

		// Act
		got, err := c.InsertVideo(context.Background(), meta, strings.NewReader(small), int64(len(small)), func(n int64) { reported = append(reported, n) })

		// Assert
		if err != nil || got.ID != "vid3" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		if len(s.startReqs()) != 2 || !bytes.Contains(s.startReqs()[1].body, []byte(small)) {
			t.Errorf("要求=%d", len(s.startReqs()))
		}
		if !slices.Equal(*slept, []time.Duration{time.Second}) {
			t.Errorf("slept=%v", *slept)
		}
		if !slices.Equal(reported, []int64{0, 0, int64(len(small))}) {
			t.Errorf("送り直すときは進み具合を 0 に戻すはず: reported=%v", reported)
		}
	})

	t.Run("再開可能アップロードを始める要求が 503 で断られたら、中身は送っていないので送り直す", func(t *testing.T) {
		// Arrange
		s := &uploadServer{
			starts: []http.HandlerFunc{refused(http.StatusServiceUnavailable, "backendError"), sessionStarted},
			chunks: []http.HandlerFunc{chunkAccepted, created("vid4")},
		}
		c, slept := newUploadClient(t, s)

		// Act
		got, err := c.InsertVideo(context.Background(), meta, bytes.NewReader(large), int64(len(large)), nil)

		// Assert
		if err != nil || got.ID != "vid4" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		if len(s.startReqs()) != 2 || len(s.chunkReqs()) != 2 || !slices.Equal(*slept, []time.Duration{time.Second}) {
			t.Errorf("最初の要求=%d 続き=%d slept=%v", len(s.startReqs()), len(s.chunkReqs()), *slept)
		}
	})

	t.Run("中身を送ったあとで接続が切れたら、送り直さずに ErrUploadUncertain を返す", func(t *testing.T) {
		// Arrange
		s := &uploadServer{starts: []http.HandlerFunc{hangUp, created("dup")}}
		c, slept := newUploadClient(t, s)

		// Act
		_, err := c.InsertVideo(context.Background(), meta, strings.NewReader(small), int64(len(small)), nil)

		// Assert
		if !errors.Is(err, domain.ErrUploadUncertain) || domain.KindOf(err) != domain.KindUnexpected {
			t.Fatalf("err=%v", err)
		}
		if len(s.startReqs()) != 1 || len(*slept) != 0 {
			t.Errorf("送り直さないはず: 要求=%d slept=%v", len(s.startReqs()), *slept)
		}
	})

	t.Run("再開可能アップロードの途中で接続が切れたら、送り直さずに ErrUploadUncertain を返す", func(t *testing.T) {
		// Arrange
		s := &uploadServer{
			starts: []http.HandlerFunc{sessionStarted},
			chunks: []http.HandlerFunc{hangUp},
		}
		c, slept := newUploadClient(t, s)

		// Act
		_, err := c.InsertVideo(context.Background(), meta, bytes.NewReader(large), int64(len(large)), nil)

		// Assert
		if !errors.Is(err, domain.ErrUploadUncertain) || domain.KindOf(err) != domain.KindUnexpected {
			t.Fatalf("err=%v", err)
		}
		if len(s.startReqs()) != 1 || len(*slept) != 0 {
			t.Errorf("送り直さないはず: 最初の要求=%d slept=%v", len(s.startReqs()), *slept)
		}
		if strings.Contains(err.Error(), "SECRET") {
			t.Errorf("upload_id を出してしまう: %v", err)
		}
	})

	t.Run("中身を含む要求が 503 で終わったら、作られたかもしれないので送り直さない", func(t *testing.T) {
		// Arrange
		s := &uploadServer{starts: []http.HandlerFunc{refused(http.StatusServiceUnavailable, "backendError"), created("dup")}}
		c, slept := newUploadClient(t, s)

		// Act
		_, err := c.InsertVideo(context.Background(), meta, strings.NewReader(small), int64(len(small)), nil)

		// Assert
		if !errors.Is(err, domain.ErrUploadUncertain) {
			t.Fatalf("err=%v", err)
		}
		if len(s.startReqs()) != 1 || len(*slept) != 0 {
			t.Errorf("要求=%d slept=%v", len(s.startReqs()), *slept)
		}
	})

	t.Run("uploadLimitExceeded は KindQuota で、送り直さない", func(t *testing.T) {
		// Arrange
		s := &uploadServer{starts: []http.HandlerFunc{refused(http.StatusForbidden, "uploadLimitExceeded"), created("dup")}}
		c, _ := newUploadClient(t, s)

		// Act
		_, err := c.InsertVideo(context.Background(), meta, strings.NewReader(small), int64(len(small)), nil)

		// Assert
		if domain.KindOf(err) != domain.KindQuota || errors.Is(err, domain.ErrUploadUncertain) {
			t.Errorf("err=%v", err)
		}
		if len(s.startReqs()) != 1 {
			t.Errorf("要求=%d", len(s.startReqs()))
		}
	})

	t.Run("400（badRequest）で返った uploadLimitExceeded も KindQuota で、送り直さない", func(t *testing.T) {
		// Arrange
		s := &uploadServer{starts: []http.HandlerFunc{refused(http.StatusBadRequest, "uploadLimitExceeded"), created("dup")}}
		c, slept := newUploadClient(t, s)

		// Act
		_, err := c.InsertVideo(context.Background(), meta, strings.NewReader(small), int64(len(small)), nil)

		// Assert
		if domain.KindOf(err) != domain.KindQuota || errors.Is(err, domain.ErrUploadUncertain) {
			t.Errorf("err=%v kind=%v", err, domain.KindOf(err))
		}
		if len(s.startReqs()) != 1 || len(*slept) != 0 {
			t.Errorf("要求=%d slept=%v", len(s.startReqs()), *slept)
		}
	})

	t.Run("要求を書いた記録がなくても YouTube の応答があれば、送り直さずに ErrUploadUncertain を返す", func(t *testing.T) {
		for _, code := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
			// Arrange
			// 要求を書いたことを net/http/httptrace に知らせない偽の通信。見張りが要求を見落としたときを真似る。
			var calls atomic.Int32
			rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				if r.Body != nil {
					_, _ = io.Copy(io.Discard, r.Body)
					_ = r.Body.Close()
				}
				reason := "backendError"
				if code == http.StatusTooManyRequests {
					reason = "rateLimitExceeded"
				}
				return &http.Response{
					StatusCode: code,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"error":{"code":%d,"message":"x","errors":[{"reason":%q,"message":"x"}]}}`, code, reason))),
					Request:    r,
				}, nil
			})
			svc, err := youtube.NewService(context.Background(), option.WithHTTPClient(&http.Client{Transport: rt}), option.WithEndpoint("http://youtube.invalid/"))
			if err != nil {
				t.Fatal(err)
			}
			c := NewClient(svc)
			var slept []time.Duration
			c.sleep = func(d time.Duration) { slept = append(slept, d) }

			// Act
			_, err = c.InsertVideo(context.Background(), meta, strings.NewReader(small), int64(len(small)), nil)

			// Assert
			if !errors.Is(err, domain.ErrUploadUncertain) {
				t.Errorf("%d: err=%v", code, err)
			}
			if calls.Load() != 1 || len(slept) != 0 {
				t.Errorf("%d: 送り直さないはず: 要求=%d slept=%v", code, calls.Load(), slept)
			}
		}
	})

	t.Run("成功の応答に動画 ID がなければ ErrUploadUncertain を返す", func(t *testing.T) {
		// Arrange
		s := &uploadServer{starts: []http.HandlerFunc{created("")}}
		c, _ := newUploadClient(t, s)

		// Act
		_, err := c.InsertVideo(context.Background(), meta, strings.NewReader(small), int64(len(small)), nil)

		// Assert
		if !errors.Is(err, domain.ErrUploadUncertain) || len(s.startReqs()) != 1 {
			t.Errorf("err=%v 要求=%d", err, len(s.startReqs()))
		}
	})

	t.Run("接続できなければ、何も送っていないので ErrUploadUncertain にしない", func(t *testing.T) {
		// Arrange
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()
		svc, err := youtube.NewService(context.Background(), option.WithHTTPClient(&http.Client{}), option.WithEndpoint(srv.URL))
		if err != nil {
			t.Fatal(err)
		}
		c := NewClient(svc)
		c.sleep = func(time.Duration) {}

		// Act
		_, err = c.InsertVideo(context.Background(), meta, strings.NewReader(small), int64(len(small)), nil)

		// Assert
		if err == nil || errors.Is(err, domain.ErrUploadUncertain) {
			t.Errorf("err=%v", err)
		}
	})

	t.Run("先頭に戻せない本文なら、429 でも送り直さない", func(t *testing.T) {
		// Arrange
		s := &uploadServer{starts: []http.HandlerFunc{refused(http.StatusTooManyRequests, "rateLimitExceeded"), created("vid5")}}
		c, slept := newUploadClient(t, s)

		// Act
		_, err := c.InsertVideo(context.Background(), meta, onlyReader{strings.NewReader(small)}, int64(len(small)), nil)

		// Assert
		if err == nil || errors.Is(err, domain.ErrUploadUncertain) {
			t.Fatalf("err=%v", err)
		}
		if len(s.startReqs()) != 1 || len(*slept) != 0 {
			t.Errorf("要求=%d slept=%v", len(s.startReqs()), *slept)
		}
	})
}

func TestSetThumbnail(t *testing.T) {
	t.Run("動画 ID を付けて画像を送る", func(t *testing.T) {
		// Arrange
		var path string
		var got sentRequest
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			path, got = r.URL.Path, sentRequest{header: r.Header.Clone(), query: r.URL.Query(), body: body}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[]}`))
		})

		// Act
		err := c.SetThumbnail(context.Background(), "vid1", strings.NewReader("PNGDATA"), "image/png")
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if path != "/upload/youtube/v3/thumbnails/set" || got.query.Get("videoId") != "vid1" {
			t.Errorf("path=%s query=%v", path, got.query)
		}
		if !bytes.Contains(got.body, []byte("PNGDATA")) || !bytes.Contains(got.body, []byte("image/png")) {
			t.Errorf("body=%q", got.body)
		}
	})

	t.Run("503 は同じ画像で送り直す", func(t *testing.T) {
		// Arrange
		var bodies [][]byte
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			bodies = append(bodies, body)
			if len(bodies) == 1 {
				refused(http.StatusServiceUnavailable, "backendError")(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[]}`))
		})

		// Act
		err := c.SetThumbnail(context.Background(), "vid1", strings.NewReader("PNGDATA"), "image/png")

		// Assert
		if err != nil || len(bodies) != 2 || !bytes.Contains(bodies[1], []byte("PNGDATA")) {
			t.Errorf("err=%v bodies=%d", err, len(bodies))
		}
	})

	t.Run("403 のクォータ切れは KindQuota", func(t *testing.T) {
		// Arrange
		c := newTestClient(t, refused(http.StatusForbidden, "quotaExceeded"))

		// Act
		err := c.SetThumbnail(context.Background(), "vid1", strings.NewReader("PNGDATA"), "image/png")

		// Assert
		if domain.KindOf(err) != domain.KindQuota {
			t.Errorf("err=%v", err)
		}
	})

	t.Run("403 の forbidden は、電話番号の確認の案内を含む KindUnexpected", func(t *testing.T) {
		// Arrange
		c := newTestClient(t, refused(http.StatusForbidden, "forbidden"))

		// Act
		err := c.SetThumbnail(context.Background(), "vid1", strings.NewReader("PNGDATA"), "image/png")

		// Assert
		if domain.KindOf(err) != domain.KindUnexpected || !strings.Contains(err.Error(), "電話番号") {
			t.Errorf("err=%v kind=%v", err, domain.KindOf(err))
		}
	})
}

// playlistJSON は、偽の YouTube が返す再生リスト。
func playlistJSON(id, title, desc, privacy string, items int) map[string]any {
	return map[string]any{
		"id":             id,
		"snippet":        map[string]any{"title": title, "description": desc},
		"status":         map[string]any{"privacyStatus": privacy},
		"contentDetails": map[string]any{"itemCount": items},
	}
}

func TestListMyPlaylists(t *testing.T) {
	t.Run("mine=true で全ページをたどり、再生リストの項目を変換して返す", func(t *testing.T) {
		// Arrange
		var queries []url.Values
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			queries = append(queries, r.URL.Query())
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("pageToken") == "" {
				_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{playlistJSON("PL1", "A", "説明\n\n[youtube-cli]", "unlisted", 3)}, "nextPageToken": "p2"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{playlistJSON("PL2", "B", "", "public", 0)}})
		})

		// Act
		got, err := c.ListMyPlaylists(context.Background())

		// Assert
		want := []domain.Playlist{
			{ID: "PL1", Title: "A", Description: "説明\n\n[youtube-cli]", PrivacyStatus: "unlisted", ItemCount: 3},
			{ID: "PL2", Title: "B", PrivacyStatus: "public"},
		}
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		q := queries[0]
		if len(queries) != 2 || q.Get("mine") != "true" || q.Get("maxResults") != "50" || strings.Join(q["part"], ",") != "snippet,status,contentDetails" || queries[1].Get("pageToken") != "p2" {
			t.Errorf("queries=%v", queries)
		}
	})

	t.Run("403 のクォータ切れは KindQuota", func(t *testing.T) {
		// Arrange
		c := newTestClient(t, refused(http.StatusForbidden, "quotaExceeded"))

		// Act
		_, err := c.ListMyPlaylists(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindQuota {
			t.Errorf("err=%v", err)
		}
	})
}

func TestGetPlaylists(t *testing.T) {
	t.Run("ID を 50 件ずつ指定して引き、見つかったものを返す", func(t *testing.T) {
		// Arrange
		var batches []int
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			batches = append(batches, len(q["id"]))
			if strings.Join(q["part"], ",") != "snippet,status,contentDetails" || q.Has("mine") || q.Has("maxResults") || q.Has("pageToken") {
				t.Errorf("ID で引くときは mine・maxResults・pageToken を送らない: query=%v", q)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{playlistJSON(q["id"][0], "A", "", "unlisted", 1)}})
		})
		ids := make([]string, 60)
		for i := range ids {
			ids[i] = fmt.Sprintf("PL%02d", i)
		}

		// Act
		got, err := c.GetPlaylists(context.Background(), ids)

		// Assert
		if err != nil || len(got) != 2 || got[0].ID != "PL00" || got[1].ID != "PL50" || got[0].ItemCount != 1 {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		if !slices.Equal(batches, []int{50, 10}) {
			t.Errorf("batches=%v", batches)
		}
	})

	t.Run("ID が空なら API を呼ばない", func(t *testing.T) {
		// Arrange
		var calls atomic.Int32
		c := newTestClient(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })

		// Act
		got, err := c.GetPlaylists(context.Background(), nil)

		// Assert
		if err != nil || len(got) != 0 || calls.Load() != 0 {
			t.Errorf("got=%v err=%v calls=%d", got, err, calls.Load())
		}
	})
}

// playlistServer は、再生リストを作る・動画を入れる要求に、responds の順で応える偽の YouTube。尽きたら最後のものを使う。
type playlistServer struct {
	mu       sync.Mutex
	responds []http.HandlerFunc
	got      []sentRequest
	paths    []string
}

func (s *playlistServer) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.got = append(s.got, sentRequest{header: r.Header.Clone(), query: r.URL.Query(), body: body})
	s.paths = append(s.paths, r.Method+" "+r.URL.Path)
	respond := s.responds[min(len(s.got), len(s.responds))-1]
	s.mu.Unlock()
	respond(w, r)
}

// count は、受け取った要求の数を返す。hangUp のように何も書かずに切る応答では、通信が前後関係を作らないので、ロックして読む。
func (s *playlistServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.got)
}

// respondJSON は、v を JSON で返す応答。
func respondJSON(v any) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
}

func TestCreatePlaylist(t *testing.T) {
	t.Run("限定公開に固定し、説明の最後に目印を足して作り、作った再生リストを返す", func(t *testing.T) {
		// Arrange
		s := &playlistServer{responds: []http.HandlerFunc{respondJSON(playlistJSON("PLnew", "Go 入門", "Go を学ぶ\n\n[youtube-cli]", "unlisted", 0))}}
		c := newTestClient(t, s.handle)

		// Act
		got, err := c.CreatePlaylist(context.Background(), "Go 入門", "Go を学ぶ")

		// Assert
		if err != nil || got.ID != "PLnew" || got.PrivacyStatus != "unlisted" || got.URL() != "https://www.youtube.com/playlist?list=PLnew" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		if len(s.got) != 1 || s.paths[0] != "POST /youtube/v3/playlists" || strings.Join(s.got[0].query["part"], ",") != "snippet,status" {
			t.Errorf("paths=%v query=%v", s.paths, s.got[0].query)
		}
		body := string(s.got[0].body)
		if !strings.Contains(body, `"privacyStatus":"unlisted"`) || !strings.Contains(body, `"title":"Go 入門"`) || !strings.Contains(body, `"description":"Go を学ぶ\n\n[youtube-cli]"`) {
			t.Errorf("body=%s", body)
		}
	})

	t.Run("説明にすでに目印があれば重ねない", func(t *testing.T) {
		// Arrange
		s := &playlistServer{responds: []http.HandlerFunc{respondJSON(playlistJSON("PLnew", "A", "[youtube-cli]", "unlisted", 0))}}
		c := newTestClient(t, s.handle)

		// Act
		_, err := c.CreatePlaylist(context.Background(), "A", "[youtube-cli]")

		// Assert
		if err != nil || !strings.Contains(string(s.got[0].body), `"description":"[youtube-cli]"`) {
			t.Errorf("err=%v body=%s", err, s.got[0].body)
		}
	})

	t.Run("429 で断られたら、何も作られていないので待って送り直す", func(t *testing.T) {
		// Arrange
		s := &playlistServer{responds: []http.HandlerFunc{refused(http.StatusTooManyRequests, "rateLimitExceeded"), respondJSON(playlistJSON("PLnew", "A", "[youtube-cli]", "unlisted", 0))}}
		c := newTestClient(t, s.handle)

		// Act
		got, err := c.CreatePlaylist(context.Background(), "A", "")

		// Assert
		if err != nil || got.ID != "PLnew" || len(s.got) != 2 {
			t.Errorf("got=%+v err=%v 要求=%d", got, err, len(s.got))
		}
	})

	t.Run("503 や通信の失敗は作られたかもしれないので送り直さず、playlist ls で確かめるよう案内する", func(t *testing.T) {
		for name, fail := range map[string]http.HandlerFunc{"503": refused(http.StatusServiceUnavailable, "backendError"), "接続が切れた": hangUp} {
			// Arrange
			s := &playlistServer{responds: []http.HandlerFunc{fail, respondJSON(playlistJSON("PLnew", "A", "[youtube-cli]", "unlisted", 0))}}
			c := newTestClient(t, s.handle)

			// Act
			_, err := c.CreatePlaylist(context.Background(), "A", "")

			// Assert
			if n := s.count(); err == nil || n != 1 {
				t.Errorf("%s: err=%v 要求=%d", name, err, n)
			}
			if domain.KindOf(err) != domain.KindUnexpected || !strings.Contains(err.Error(), "できているかもしれません") || !strings.Contains(err.Error(), "`youtube-cli playlist ls`") {
				t.Errorf("%s: err=%v", name, err)
			}
		}
	})

	t.Run("4xx で断られたら何も作られていないので、確かめる案内は出さない", func(t *testing.T) {
		// Arrange
		s := &playlistServer{responds: []http.HandlerFunc{refused(http.StatusBadRequest, "invalidPlaylistSnippet")}}
		c := newTestClient(t, s.handle)

		// Act
		_, err := c.CreatePlaylist(context.Background(), "A", "")

		// Assert
		if err == nil || strings.Contains(err.Error(), "playlist ls") {
			t.Errorf("err=%v", err)
		}
	})
}

func TestAddToPlaylist(t *testing.T) {
	t.Run("再生リストの ID と動画の ID を付けて入れる", func(t *testing.T) {
		// Arrange
		s := &playlistServer{responds: []http.HandlerFunc{respondJSON(map[string]any{"id": "item1"})}}
		c := newTestClient(t, s.handle)

		// Act
		err := c.AddToPlaylist(context.Background(), "PL1", "vid1")

		// Assert
		if err != nil || len(s.got) != 1 || s.paths[0] != "POST /youtube/v3/playlistItems" || s.got[0].query.Get("part") != "snippet" {
			t.Fatalf("err=%v paths=%v", err, s.paths)
		}
		var sent struct {
			Snippet struct {
				PlaylistID string `json:"playlistId"`
				ResourceID struct {
					Kind    string `json:"kind"`
					VideoID string `json:"videoId"`
				} `json:"resourceId"`
			} `json:"snippet"`
		}
		if err := json.Unmarshal(s.got[0].body, &sent); err != nil {
			t.Fatal(err)
		}
		if sent.Snippet.PlaylistID != "PL1" || sent.Snippet.ResourceID.Kind != "youtube#video" || sent.Snippet.ResourceID.VideoID != "vid1" {
			t.Errorf("sent=%+v", sent)
		}
	})

	t.Run("503 や通信の失敗は入ったかもしれないので送り直さず、ls --playlist で確かめるよう案内する", func(t *testing.T) {
		for name, fail := range map[string]http.HandlerFunc{"503": refused(http.StatusServiceUnavailable, "backendError"), "接続が切れた": hangUp} {
			// Arrange
			s := &playlistServer{responds: []http.HandlerFunc{fail, respondJSON(map[string]any{"id": "item1"})}}
			c := newTestClient(t, s.handle)

			// Act
			err := c.AddToPlaylist(context.Background(), "PL1", "vid1")

			// Assert
			if n := s.count(); err == nil || n != 1 {
				t.Errorf("%s: err=%v 要求=%d", name, err, n)
			}
			if domain.KindOf(err) != domain.KindUnexpected || !strings.Contains(err.Error(), "入っているかもしれません") || !strings.Contains(err.Error(), "`youtube-cli ls --playlist PL1`") {
				t.Errorf("%s: err=%v", name, err)
			}
		}
	})

	t.Run("429 で断られ続けたら何も入っていないので、確かめる案内は出さない", func(t *testing.T) {
		// Arrange
		c := newTestClient(t, refused(http.StatusTooManyRequests, "rateLimitExceeded"))

		// Act
		err := c.AddToPlaylist(context.Background(), "PL1", "vid1")

		// Assert
		if err == nil || strings.Contains(err.Error(), "ls --playlist") {
			t.Errorf("err=%v", err)
		}
	})

	t.Run("404 は KindNotFound", func(t *testing.T) {
		// Arrange
		c := newTestClient(t, refused(http.StatusNotFound, "playlistNotFound"))

		// Act
		err := c.AddToPlaylist(context.Background(), "PL1", "vid1")

		// Assert
		if domain.KindOf(err) != domain.KindNotFound {
			t.Errorf("err=%v", err)
		}
	})
}
