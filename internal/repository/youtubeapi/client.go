// Package youtubeapi は、YouTube Data API の呼び出し（usecase.YouTube の実装）と、失敗したときのやり直しを持つ。
package youtubeapi

import (
	"context"
	"slices"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/repository/auth"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

const maxAttempts = 5

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
	return mapError(c.retry(ctx, fn))
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
