// Package youtubeapi は、YouTube Data API の呼び出し（usecase.YouTube の実装）と、失敗したときのやり直しを持つ。
package youtubeapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/repository/auth"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

const (
	maxAttempts = 5
	// uploadChunk は、再開可能アップロードで 1 回に送る大きさ（16 MiB）。これより小さい動画は、ライブラリが 1 回の要求（multipart）で送る。
	uploadChunk = 16 << 20
	// chunkRetry は、再開可能アップロードの塊 1 つを、ライブラリが送り直し続ける時間の上限。
	chunkRetry = 2 * time.Minute
)

// Client は、youtube/v3 で usecase.YouTube を実装する。
type Client struct {
	svc      *youtube.Service
	sleep    func(time.Duration)
	attempts int
}

var _ usecase.YouTube = (*Client)(nil)

// NewClient は、svc を使う Client を返す。
func NewClient(svc *youtube.Service) *Client {
	return &Client{svc: svc, sleep: time.Sleep, attempts: maxAttempts}
}

// NewOpener は、保存済みのトークンで YouTube を開く関数を返す。
func NewOpener(a *auth.Auth) usecase.YouTubeOpener {
	return func(ctx context.Context) (usecase.YouTube, error) {
		ts, err := a.TokenSource(ctx)
		if err != nil {
			return nil, err
		}
		svc, err := youtube.NewService(ctx, option.WithTokenSource(ts))
		if err != nil {
			return nil, domain.Errorf(domain.KindUnexpected, "YouTube に接続できません").Wrap(err)
		}
		return NewClient(svc), nil
	}
}

// do は、fn をやり直しつきで呼び、エラーを変換する。
func (c *Client) do(ctx context.Context, fn func() error) error {
	return mapError(c.retry(ctx, isRetryable, fn))
}

// doCreate は、何かを作る fn を、混み合いで断られたとき（何も作られていない）だけやり直して呼び、エラーを変換する。
// 5xx や通信の失敗は、YouTube が作ったあとで失敗したのかもしれないので、二重に作らないようやり直さない。
func (c *Client) doCreate(ctx context.Context, fn func() error) error {
	return mapError(c.retry(ctx, isRateLimited, fn))
}

// MyChannel は、ログイン中のチャンネルを返す。チャンネルがなければ KindSetup。
func (c *Client) MyChannel(ctx context.Context) (domain.Channel, error) {
	var resp *youtube.ChannelListResponse
	err := c.do(ctx, func() (err error) {
		resp, err = c.svc.Channels.List([]string{"snippet", "contentDetails"}).Mine(true).Context(ctx).Do()
		return err
	})
	if err != nil {
		return domain.Channel{}, err
	}
	if len(resp.Items) == 0 {
		return domain.Channel{}, domain.Errorf(domain.KindSetup, "このアカウントには YouTube のチャンネルがありません。YouTube でチャンネルを作ってから、`youtube-cli login` をやり直してください")
	}
	item := resp.Items[0]
	ch := domain.Channel{ID: item.Id}
	if item.Snippet != nil {
		ch.Title = item.Snippet.Title
	}
	if item.ContentDetails != nil && item.ContentDetails.RelatedPlaylists != nil {
		ch.UploadsPlaylistID = item.ContentDetails.RelatedPlaylists.Uploads
	}
	return ch, nil
}

// pageSize は、一覧と一括取得で1回に頼む最大の件数（API の上限）。
const pageSize = 50

// ListPlaylistVideoIDs は、再生リストの動画の ID を、全ページ分返す。
func (c *Client) ListPlaylistVideoIDs(ctx context.Context, playlistID string) ([]string, error) {
	var ids []string
	pageToken := ""
	for {
		var resp *youtube.PlaylistItemListResponse
		err := c.do(ctx, func() (err error) {
			resp, err = c.svc.PlaylistItems.List([]string{"contentDetails"}).PlaylistId(playlistID).MaxResults(pageSize).PageToken(pageToken).Context(ctx).Do()
			return err
		})
		if err != nil {
			return nil, err
		}
		for _, it := range resp.Items {
			if it.ContentDetails != nil && it.ContentDetails.VideoId != "" {
				ids = append(ids, it.ContentDetails.VideoId)
			}
		}
		if resp.NextPageToken == "" {
			return ids, nil
		}
		pageToken = resp.NextPageToken
	}
}

// GetVideos は、ID の動画を50件ずつ引いて返す。見つからない ID は結果に入れない。
func (c *Client) GetVideos(ctx context.Context, ids []string) ([]domain.Video, error) {
	var videos []domain.Video
	for batch := range slices.Chunk(ids, pageSize) {
		var resp *youtube.VideoListResponse
		err := c.do(ctx, func() (err error) {
			resp, err = c.svc.Videos.List([]string{"snippet", "status"}).Id(batch...).Context(ctx).Do()
			return err
		})
		if err != nil {
			return nil, err
		}
		for _, it := range resp.Items {
			videos = append(videos, toVideo(it))
		}
	}
	return videos, nil
}

func toVideo(it *youtube.Video) domain.Video {
	v := domain.Video{ID: it.Id}
	if s := it.Snippet; s != nil {
		v.Title, v.Description, v.Tags = s.Title, s.Description, s.Tags
		v.PublishedAt, _ = time.Parse(time.RFC3339, s.PublishedAt)
	}
	if s := it.Status; s != nil {
		v.PrivacyStatus, v.UploadStatus = s.PrivacyStatus, s.UploadStatus
	}
	return v
}

// InsertVideo は、body（size バイト）を限定公開の動画として上げ、作った動画を返す。目印のタグは必ず付ける。
// uploadChunk 以上の動画は再開可能アップロードで分けて送り、送り始めたあとの塊ごとの一時的な失敗は、
// ライブラリが同じセッションの続きから送り直す（chunkRetry まで）。
//
// 失敗したとき、最初から新しく上げ直すのは、動画ができていないと言い切れるとき（sendWatch.maybeCreated が false）だけ。
// そのうえで、混み合いなどのやり直せる失敗で、body が io.Seeker で先頭に戻せれば、1s、2s、4s、8s と待って最大 attempts 回まで送り直す。
// 動画ができたかもしれないときは、二重に上げないよう送り直さずに domain.ErrUploadUncertain を返す。
func (c *Client) InsertVideo(ctx context.Context, m video.Meta, body io.Reader, size int64, progress func(sent int64)) (domain.Video, error) {
	report := func(sent int64) {
		if progress != nil {
			progress(sent)
		}
	}
	seeker, seekable := body.(io.Seeker)
	for attempt := 1; ; attempt++ {
		report(0)
		var w sendWatch
		res, err := c.insertOnce(w.watch(ctx), m, body, progress)
		if err == nil && res.Id != "" {
			report(size)
			return toVideo(res), nil
		}
		if err == nil {
			return domain.Video{}, domain.UploadUncertain(errors.New("YouTube の応答に動画 ID がありません"))
		}
		if w.maybeCreated(err) {
			redactURL(err)
			return domain.Video{}, domain.UploadUncertain(err)
		}
		if !isRetryable(err) || !seekable || attempt >= c.attempts || ctx.Err() != nil {
			return domain.Video{}, mapError(err)
		}
		c.sleep(backoff(attempt))
		if _, serr := seeker.Seek(0, io.SeekStart); serr != nil {
			return domain.Video{}, domain.Errorf(domain.KindUnexpected, "動画のファイルを最初から読み直せません").Wrap(serr)
		}
	}
}

// insertOnce は、Videos.Insert を 1 回呼ぶ。公開範囲は限定公開に固定する。子ども向けと合成の申告は、
// false でも ForceSendFields で明示して送る（省くと JSON から落ち、申告しなかったことになるため）。
func (c *Client) insertOnce(ctx context.Context, m video.Meta, body io.Reader, progress func(sent int64)) (*youtube.Video, error) {
	v := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       m.Title,
			Description: m.Description,
			Tags:        video.WithMarker(m.Tags),
			CategoryId:  m.CategoryID,
		},
		Status: &youtube.VideoStatus{
			PrivacyStatus:           video.PrivacyUnlisted,
			SelfDeclaredMadeForKids: m.MadeForKids,
			ContainsSyntheticMedia:  m.SyntheticMedia,
			ForceSendFields:         []string{"SelfDeclaredMadeForKids", "ContainsSyntheticMedia"},
		},
	}
	call := c.svc.Videos.Insert([]string{"snippet", "status"}, v).
		Media(body, googleapi.ChunkSize(uploadChunk), googleapi.ChunkRetryDeadline(chunkRetry)).
		Context(ctx)
	if progress != nil {
		call = call.ProgressUpdater(func(current, _ int64) { progress(current) })
	}
	return call.Do()
}

// sendWatch は、Videos.Insert の Do を 1 回呼ぶ間に YouTube へ書いた要求を、net/http/httptrace で見張る。
// 失敗したとき、YouTube に動画ができたかもしれないかを見分けるために使う。
//
// Do の中の要求の流れは、動画の大きさで 2 通りある。
//   - uploadChunk より小さい: 動画の項目と中身を、1 回の要求（multipart）で送る
//   - uploadChunk 以上: まず項目だけの要求で再開可能アップロードのセッションを作り（X-Upload-Content-Type ヘッダーが付く）、
//     応答の Location へ、中身を塊ごとの要求で送る
//
// 要求の数は、ヘッダーを書き終えた回数（WroteHeaders）で数える。net/http が使い古しの接続で失敗した要求を
// 内部で送り直したときは多めに数えるが、多めに数えても「できたかもしれない」側に倒れるだけで、二重には上げない。
type sendWatch struct {
	wrote   atomic.Int32 // ヘッダーを書き終えた要求の数
	session atomic.Bool  // 最初の要求が、セッションを作る要求（動画の中身を含まない）だった
}

// watch は、ctx に見張りを付けて返す。
func (w *sendWatch) watch(ctx context.Context) context.Context {
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		WroteHeaderField: func(key string, _ []string) {
			if w.wrote.Load() == 0 && strings.EqualFold(key, "X-Upload-Content-Type") {
				w.session.Store(true)
			}
		},
		WroteHeaders: func() { w.wrote.Add(1) },
	})
}

// maybeCreated は、Do が err で失敗したとき、YouTube に動画ができたかもしれないかを返す。
// できていないと言い切れるのは、次の 2 つのときだけ。
//   - 要求を 1 つも書いていない（接続できなかった、トークンを更新できなかったなど）。
//   - 書いた要求は最初の 1 つだけで、YouTube がそれに失敗の状態コードで応えた（err が googleapi.Error）。
//     ただし中身を含む要求（multipart）なら 4xx のときだけ。5xx は、作ったあとで失敗したのかもしれない。
//     セッションを作る要求は中身を含まず、断られればセッションの送り先も受け取っていないので、5xx でも何もできていない。
//
// 2 つ目の要求を書いた（セッションができて中身を送り始めた）とき、最初の要求の応答を受け取れなかった（接続が切れたなど）とき、
// セッションを作れたのに続きを書く前に失敗したとき、成功の応答を読めなかったときは、できたかもしれない。
// 2 つ目以降の要求への応答は 2 つ目の要求を書いたあとにしか来ないので、要求が 1 つのときの googleapi.Error は最初の要求への応答。
func (w *sendWatch) maybeCreated(err error) bool {
	switch w.wrote.Load() {
	case 0:
		return false
	case 1:
		gerr, ok := errors.AsType[*googleapi.Error](err)
		if !ok {
			return true
		}
		return !w.session.Load() && gerr.Code >= 500
	default:
		return true
	}
}

// playlistParts は、再生リストを引くときに頼む部分（名前と説明、公開範囲、動画の数）。
var playlistParts = []string{"snippet", "status", "contentDetails"}

// ListMyPlaylists は、ログイン中のチャンネルの再生リストを、全ページ分返す。
func (c *Client) ListMyPlaylists(ctx context.Context) ([]domain.Playlist, error) {
	return c.listPlaylists(ctx, func() *youtube.PlaylistsListCall {
		return c.svc.Playlists.List(playlistParts).Mine(true)
	})
}

// GetPlaylists は、ID の再生リストを50件ずつ引いて返す。見つからない ID は結果に入れない。
func (c *Client) GetPlaylists(ctx context.Context, ids []string) ([]domain.Playlist, error) {
	var playlists []domain.Playlist
	for batch := range slices.Chunk(ids, pageSize) {
		got, err := c.listPlaylists(ctx, func() *youtube.PlaylistsListCall {
			return c.svc.Playlists.List(playlistParts).Id(batch...)
		})
		if err != nil {
			return nil, err
		}
		playlists = append(playlists, got...)
	}
	return playlists, nil
}

// listPlaylists は、newCall で作った Playlists.List を、1回に pageSize 件ずつ、全ページ分たどる。
func (c *Client) listPlaylists(ctx context.Context, newCall func() *youtube.PlaylistsListCall) ([]domain.Playlist, error) {
	var playlists []domain.Playlist
	pageToken := ""
	for {
		var resp *youtube.PlaylistListResponse
		err := c.do(ctx, func() (err error) {
			resp, err = newCall().MaxResults(pageSize).PageToken(pageToken).Context(ctx).Do()
			return err
		})
		if err != nil {
			return nil, err
		}
		for _, it := range resp.Items {
			playlists = append(playlists, toPlaylist(it))
		}
		if resp.NextPageToken == "" {
			return playlists, nil
		}
		pageToken = resp.NextPageToken
	}
}

func toPlaylist(it *youtube.Playlist) domain.Playlist {
	p := domain.Playlist{ID: it.Id}
	if s := it.Snippet; s != nil {
		p.Title, p.Description = s.Title, s.Description
	}
	if s := it.Status; s != nil {
		p.PrivacyStatus = s.PrivacyStatus
	}
	if d := it.ContentDetails; d != nil {
		p.ItemCount = d.ItemCount
	}
	return p
}

// CreatePlaylist は、限定公開の再生リストを作り、作った再生リストを返す。説明の最後に目印がなければ足す。
// 混み合いで断られたとき（何も作られていない）だけ送り直す。5xx や通信の失敗は、作られたかもしれないので送り直さない。
func (c *Client) CreatePlaylist(ctx context.Context, title, description string) (domain.Playlist, error) {
	if !video.HasPlaylistMarker(description) {
		description = video.AddPlaylistMarker(description)
	}
	pl := &youtube.Playlist{
		Snippet: &youtube.PlaylistSnippet{Title: title, Description: description},
		Status:  &youtube.PlaylistStatus{PrivacyStatus: video.PrivacyUnlisted},
	}
	var res *youtube.Playlist
	err := c.doCreate(ctx, func() (err error) {
		res, err = c.svc.Playlists.Insert([]string{"snippet", "status"}, pl).Context(ctx).Do()
		return err
	})
	if err != nil {
		return domain.Playlist{}, err
	}
	return toPlaylist(res), nil
}

// AddToPlaylist は、再生リストの最後に動画を入れる。
// 混み合いで断られたとき（何も入っていない）だけ送り直す。5xx や通信の失敗は、入ったかもしれないので送り直さない。
func (c *Client) AddToPlaylist(ctx context.Context, playlistID, videoID string) error {
	item := &youtube.PlaylistItem{Snippet: &youtube.PlaylistItemSnippet{
		PlaylistId: playlistID,
		ResourceId: &youtube.ResourceId{Kind: "youtube#video", VideoId: videoID},
	}}
	return c.doCreate(ctx, func() error {
		_, err := c.svc.PlaylistItems.Insert([]string{"snippet"}, item).Context(ctx).Do()
		return err
	})
}

// SetThumbnail は、動画のサムネイルを img にする。やり直すときに同じ画像を送れるよう、先に読み込む（画像は 2 MiB まで）。
func (c *Client) SetThumbnail(ctx context.Context, videoID string, img io.Reader, mimeType string) error {
	data, err := io.ReadAll(img)
	if err != nil {
		return domain.Errorf(domain.KindUnexpected, "サムネイルの画像を読めません").Wrap(err)
	}
	err = c.do(ctx, func() error {
		_, err := c.svc.Thumbnails.Set(videoID).Media(bytes.NewReader(data), googleapi.ContentType(mimeType)).Context(ctx).Do()
		return err
	})
	if gerr, ok := errors.AsType[*googleapi.Error](err); ok && gerr.Code == http.StatusForbidden && domain.KindOf(err) == domain.KindUnexpected {
		return domain.Errorf(domain.KindUnexpected,
			"YouTube がサムネイルの設定を断りました。チャンネルの電話番号の確認が済んでいない可能性があります（https://www.youtube.com/verify で確認できます）").Wrap(err)
	}
	return err
}
