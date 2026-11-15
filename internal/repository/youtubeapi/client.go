// Package youtubeapi は、YouTube Data API の呼び出し（usecase.YouTube の実装）と、失敗したときのやり直しを持つ。
package youtubeapi

import (
	"context"
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
