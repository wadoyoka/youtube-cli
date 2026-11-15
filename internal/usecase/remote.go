package usecase

import (
	"context"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

// Remote は、YouTube に接続するための部品。
type Remote struct {
	Open   YouTubeOpener
	State  StateStore
	Locker Locker
}

// Session は、接続した YouTube と、そのチャンネル。
type Session struct {
	YT      YouTube
	Channel domain.Channel
}

// Connect は、YouTube を開き、トークンのチャンネルが保存したチャンネルと同じかを確かめて Session を返す。
// 保存したチャンネルがない、または違うときは KindSetup（login のやり直しを案内する）。
func (r Remote) Connect(ctx context.Context) (*Session, error) {
	saved, found, err := r.State.Channel()
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, domain.Errorf(domain.KindSetup, "チャンネルが保存されていません。`youtube-cli login` を実行してください")
	}
	yt, err := r.Open(ctx)
	if err != nil {
		return nil, err
	}
	ch, err := yt.MyChannel(ctx)
	if err != nil {
		return nil, err
	}
	if ch.ID != saved.ID {
		return nil, domain.Errorf(domain.KindSetup, "ログイン中のチャンネル（%s）が、保存したチャンネル（%s）と違います。`youtube-cli login` をやり直してください", ch.Title, saved.Title)
	}
	return &Session{YT: yt, Channel: ch}, nil
}
