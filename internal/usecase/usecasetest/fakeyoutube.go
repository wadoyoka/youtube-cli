package usecasetest

import (
	"context"
	"errors"
	"io"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// YouTube は、決まった値を返す偽の YouTube。
type YouTube struct {
	Channel    domain.Channel
	ChannelErr error // MyChannel が返すエラー（Channel より優先）
	Calls      []string

	PlaylistVideoIDs map[string][]string // 再生リストの ID ごとの、動画の ID（一覧の遅れを真似るため、Videos より少なくしてよい）
	Videos           []domain.Video      // GetVideos が探す動画
	ListErr          error               // ListPlaylistVideoIDs が返すエラー
	GetErr           error               // GetVideos が返すエラー

	Inserted     []InsertedVideo // InsertVideo に渡されたもの
	InsertResult domain.Video    // InsertVideo が返す動画
	InsertErr    error           // InsertVideo が返すエラー（InsertResult より優先）

	Thumbnails   []SetThumbnailCall // SetThumbnail に渡されたもの
	ThumbnailErr error              // SetThumbnail が返すエラー
}

// InsertedVideo は、InsertVideo に渡されたもの。
type InsertedVideo struct {
	Meta video.Meta
	Body string
	Size int64
}

// SetThumbnailCall は、SetThumbnail に渡されたもの。
type SetThumbnailCall struct {
	VideoID  string
	Image    string
	MimeType string
}

// MyChannel は、ChannelErr があればそれを、なければ Channel を返す。
func (f *YouTube) MyChannel(context.Context) (domain.Channel, error) {
	f.Calls = append(f.Calls, "MyChannel")
	if f.ChannelErr != nil {
		return domain.Channel{}, f.ChannelErr
	}
	return f.Channel, nil
}

// ListPlaylistVideoIDs は、PlaylistVideoIDs の playlistID の分を返す。
func (f *YouTube) ListPlaylistVideoIDs(_ context.Context, playlistID string) ([]string, error) {
	f.Calls = append(f.Calls, "ListPlaylistVideoIDs")
	if f.ListErr != nil {
		return nil, f.ListErr
	}
	return f.PlaylistVideoIDs[playlistID], nil
}

// GetVideos は、Videos のうち ids にあるものを、ids の順に返す。
func (f *YouTube) GetVideos(_ context.Context, ids []string) ([]domain.Video, error) {
	f.Calls = append(f.Calls, "GetVideos")
	if f.GetErr != nil {
		return nil, f.GetErr
	}
	var out []domain.Video
	for _, id := range ids {
		for _, v := range f.Videos {
			if v.ID == id {
				out = append(out, v)
			}
		}
	}
	return out, nil
}

// InsertVideo は、渡されたものを記録し、中身を読み終えたら progress に知らせて、InsertErr か InsertResult を返す。
func (f *YouTube) InsertVideo(_ context.Context, m video.Meta, body io.Reader, size int64, progress func(sent int64)) (domain.Video, error) {
	f.Calls = append(f.Calls, "InsertVideo")
	b, err := io.ReadAll(body)
	if err != nil {
		return domain.Video{}, err
	}
	f.Inserted = append(f.Inserted, InsertedVideo{Meta: m, Body: string(b), Size: size})
	if f.InsertErr != nil {
		return domain.Video{}, f.InsertErr
	}
	if progress != nil {
		progress(size)
	}
	return f.InsertResult, nil
}

// SetThumbnail は、渡されたものを記録し、ThumbnailErr を返す。
func (f *YouTube) SetThumbnail(_ context.Context, videoID string, img io.Reader, mimeType string) error {
	f.Calls = append(f.Calls, "SetThumbnail")
	b, err := io.ReadAll(img)
	if err != nil {
		return err
	}
	f.Thumbnails = append(f.Thumbnails, SetThumbnailCall{VideoID: videoID, Image: string(b), MimeType: mimeType})
	return f.ThumbnailErr
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

	ChannelErr error // Channel が返すエラー
	SetErr     error // SetChannel が返すエラー（保存しない）
}

// Channel は、保存したチャンネルを返す。
func (s *State) Channel() (domain.Channel, bool, error) {
	if s.ChannelErr != nil {
		return domain.Channel{}, false, s.ChannelErr
	}
	return s.Saved, s.Found, nil
}

// SetChannel は、チャンネルを保存する。
func (s *State) SetChannel(ch domain.Channel) error {
	if s.SetErr != nil {
		return s.SetErr
	}
	s.Saved, s.Found = ch, true
	s.Sets++
	return nil
}

// Locker は、ロックの回数を数える偽の Locker。
// 本物のロック（ファイルのロック）は、同じプロセスからでも、ロック中にもう一度ロックすると待ち続ける。
// 偽物では待つ代わりに ErrRelocked を返し、ロック中にロックを取るもの（Catalog など）を呼んでしまう誤りを見つける。
type Locker struct {
	Locks   int
	LockErr error // Lock が返すエラー
	held    bool
}

// ErrRelocked は、ロック中にもう一度ロックしようとしたときに Locker が返すエラー。
var ErrRelocked = errors.New("ロック中にもう一度ロックしようとしました（本物のロックなら待ち続けます）")

// Lock は、回数を数えて、すぐに返す。ロック中なら ErrRelocked を返す。
func (l *Locker) Lock(context.Context) (func(), error) {
	if l.LockErr != nil {
		return nil, l.LockErr
	}
	if l.held {
		return nil, ErrRelocked
	}
	l.Locks++
	l.held = true
	return func() { l.held = false }, nil
}

// Ledger は、メモリ上の Ledger。
type Ledger struct {
	VideoList    []usecase.LedgerVideo
	PlaylistList []usecase.LedgerPlaylist
	Clears       int   // Clear を呼んだ回数
	ClearErr     error // Clear が返すエラー（空にしない）
	AddErr       error // AddVideo が返すエラー（足さない）
}

// Videos は、記録した動画を返す。
func (l *Ledger) Videos() ([]usecase.LedgerVideo, error) { return l.VideoList, nil }

// AddVideo は、同じ ID がなければ動画を足す。
func (l *Ledger) AddVideo(v usecase.LedgerVideo) error {
	if l.AddErr != nil {
		return l.AddErr
	}
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
	if l.ClearErr != nil {
		return l.ClearErr
	}
	l.VideoList, l.PlaylistList = nil, nil
	l.Clears++
	return nil
}
