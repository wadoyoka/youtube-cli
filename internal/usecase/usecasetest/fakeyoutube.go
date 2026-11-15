package usecasetest

import (
	"context"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// YouTube は、決まった値を返す偽の YouTube。
type YouTube struct {
	Channel    domain.Channel
	ChannelErr error // MyChannel が返すエラー（Channel より優先）
	Calls      []string
}

// MyChannel は、ChannelErr があればそれを、なければ Channel を返す。
func (f *YouTube) MyChannel(context.Context) (domain.Channel, error) {
	f.Calls = append(f.Calls, "MyChannel")
	if f.ChannelErr != nil {
		return domain.Channel{}, f.ChannelErr
	}
	return f.Channel, nil
}

// Opener は、yt を返す YouTubeOpener を作る。
func Opener(yt usecase.YouTube) usecase.YouTubeOpener {
	return func(context.Context) (usecase.YouTube, error) { return yt, nil }
}

// State は、メモリ上の StateStore。
type State struct {
	Saved domain.Channel
	Found bool
	Sets  int // SetChannel を呼んだ回数
}

// Channel は、保存したチャンネルを返す。
func (s *State) Channel() (domain.Channel, bool, error) { return s.Saved, s.Found, nil }

// SetChannel は、チャンネルを保存する。
func (s *State) SetChannel(ch domain.Channel) error {
	s.Saved, s.Found = ch, true
	s.Sets++
	return nil
}

// Locker は、何もしない Locker。ロックの回数を数える。
type Locker struct{ Locks int }

// Lock は、回数を数えるだけで、すぐに返す。
func (l *Locker) Lock(context.Context) (func(), error) {
	l.Locks++
	return func() {}, nil
}

// Ledger は、メモリ上の Ledger。
type Ledger struct {
	VideoList    []usecase.LedgerVideo
	PlaylistList []usecase.LedgerPlaylist
	Clears       int // Clear を呼んだ回数
}

// Videos は、記録した動画を返す。
func (l *Ledger) Videos() ([]usecase.LedgerVideo, error) { return l.VideoList, nil }

// AddVideo は、同じ ID がなければ動画を足す。
func (l *Ledger) AddVideo(v usecase.LedgerVideo) error {
	for _, x := range l.VideoList {
		if x.ID == v.ID {
			return nil
		}
	}
	l.VideoList = append(l.VideoList, v)
	return nil
}

// Playlists は、記録した再生リストを返す。
func (l *Ledger) Playlists() ([]usecase.LedgerPlaylist, error) { return l.PlaylistList, nil }

// AddPlaylist は、同じ ID がなければ再生リストを足す。
func (l *Ledger) AddPlaylist(p usecase.LedgerPlaylist) error {
	for _, x := range l.PlaylistList {
		if x.ID == p.ID {
			return nil
		}
	}
	l.PlaylistList = append(l.PlaylistList, p)
	return nil
}

// Clear は、記録を空にして、回数を数える。
func (l *Ledger) Clear() error {
	l.VideoList, l.PlaylistList = nil, nil
	l.Clears++
	return nil
}
