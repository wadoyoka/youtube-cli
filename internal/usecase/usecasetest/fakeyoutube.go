package usecasetest

import (
	"context"
	"errors"
	"io"
	"slices"

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

	Playlists      []domain.Playlist // ListMyPlaylists が返す再生リスト
	LatePlaylists  []domain.Playlist // 作った直後で、ListMyPlaylists にはまだ出ない再生リスト。GetPlaylists は Playlists とこれから探す
	PlaylistsErr   error             // ListMyPlaylists が返すエラー
	GotPlaylistIDs []string          // GetPlaylists に渡された ID

	Created      []CreatePlaylistCall // CreatePlaylist に渡されたもの
	CreateResult domain.Playlist      // CreatePlaylist が返す再生リスト
	CreateErr    error                // CreatePlaylist が返すエラー（CreateResult より優先）

	AddedToPlaylist  []AddToPlaylistCall // AddToPlaylist で入れたもの
	AddToPlaylistErr error               // AddToPlaylist が返すエラー（入れない）
}

// CreatePlaylistCall は、CreatePlaylist に渡されたもの。
type CreatePlaylistCall struct {
	Title       string
	Description string
}

// AddToPlaylistCall は、AddToPlaylist に渡されたもの。
type AddToPlaylistCall struct {
	PlaylistID string
	VideoID    string
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

// ListMyPlaylists は、PlaylistsErr があればそれを、なければ Playlists を返す。
func (f *YouTube) ListMyPlaylists(context.Context) ([]domain.Playlist, error) {
	f.Calls = append(f.Calls, "ListMyPlaylists")
	if f.PlaylistsErr != nil {
		return nil, f.PlaylistsErr
	}
	return f.Playlists, nil
}

// GetPlaylists は、Playlists と LatePlaylists のうち ids にあるものを、ids の順に返す。
func (f *YouTube) GetPlaylists(_ context.Context, ids []string) ([]domain.Playlist, error) {
	f.Calls = append(f.Calls, "GetPlaylists")
	f.GotPlaylistIDs = append(f.GotPlaylistIDs, ids...)
	all := append(slices.Clone(f.Playlists), f.LatePlaylists...)
	var out []domain.Playlist
	for _, id := range ids {
		for _, p := range all {
			if p.ID == id {
				out = append(out, p)
				break
			}
		}
	}
	return out, nil
}

// CreatePlaylist は、渡されたものを記録し、CreateErr か CreateResult を返す。
func (f *YouTube) CreatePlaylist(_ context.Context, title, description string) (domain.Playlist, error) {
	f.Calls = append(f.Calls, "CreatePlaylist")
	f.Created = append(f.Created, CreatePlaylistCall{Title: title, Description: description})
	if f.CreateErr != nil {
		return domain.Playlist{}, f.CreateErr
	}
	return f.CreateResult, nil
}

// AddToPlaylist は、AddToPlaylistErr がなければ、動画を PlaylistVideoIDs の再生リストの最後に入れて記録する。
func (f *YouTube) AddToPlaylist(_ context.Context, playlistID, videoID string) error {
	f.Calls = append(f.Calls, "AddToPlaylist")
	if f.AddToPlaylistErr != nil {
		return f.AddToPlaylistErr
	}
	if f.PlaylistVideoIDs == nil {
		f.PlaylistVideoIDs = map[string][]string{}
	}
	f.PlaylistVideoIDs[playlistID] = append(f.PlaylistVideoIDs[playlistID], videoID)
	f.AddedToPlaylist = append(f.AddedToPlaylist, AddToPlaylistCall{PlaylistID: playlistID, VideoID: videoID})
	return nil
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
	AddErr       error // AddVideo と AddPlaylist が返すエラー（足さない）
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
	if l.AddErr != nil {
		return l.AddErr
	}
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
