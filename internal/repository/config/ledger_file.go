package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/wadoyoka/youtube-cli/internal/repository/atomicfile"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// LedgerFile は、設定ディレクトリの ledger.json。この PC で上げた動画と作った再生リストを覚える。
// 読み書きは、呼ぶ側が Locker で囲む。
type LedgerFile struct {
	path string
}

type ledgerData struct {
	Videos    []usecase.LedgerVideo    `json:"videos"`
	Playlists []usecase.LedgerPlaylist `json:"playlists"`
}

// NewLedgerFile は、dir/ledger.json を扱う LedgerFile を作る。
func NewLedgerFile(dir string) *LedgerFile {
	return &LedgerFile{path: filepath.Join(dir, "ledger.json")}
}

func (l *LedgerFile) load() (ledgerData, error) {
	data, err := os.ReadFile(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return ledgerData{}, nil
	}
	if err != nil {
		return ledgerData{}, fmt.Errorf("記録ファイルを読めません: %w", err)
	}
	var d ledgerData
	if err := json.Unmarshal(data, &d); err != nil {
		return ledgerData{}, fmt.Errorf("記録ファイルが壊れています（%s）: %w", l.path, err)
	}
	return d, nil
}

func (l *LedgerFile) save(d ledgerData) error {
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return atomicfile.Write(l.path, data, 0o600)
}

// Videos は、記録した動画を返す。
func (l *LedgerFile) Videos() ([]usecase.LedgerVideo, error) {
	d, err := l.load()
	return d.Videos, err
}

// Playlists は、記録した再生リストを返す。
func (l *LedgerFile) Playlists() ([]usecase.LedgerPlaylist, error) {
	d, err := l.load()
	return d.Playlists, err
}

// AddVideo は、動画を記録する。同じ ID があれば足さない。
func (l *LedgerFile) AddVideo(v usecase.LedgerVideo) error {
	d, err := l.load()
	if err != nil {
		return err
	}
	if slices.ContainsFunc(d.Videos, func(x usecase.LedgerVideo) bool { return x.ID == v.ID }) {
		return nil
	}
	d.Videos = append(d.Videos, v)
	return l.save(d)
}

// AddPlaylist は、再生リストを記録する。同じ ID があれば足さない。
func (l *LedgerFile) AddPlaylist(p usecase.LedgerPlaylist) error {
	d, err := l.load()
	if err != nil {
		return err
	}
	if slices.ContainsFunc(d.Playlists, func(x usecase.LedgerPlaylist) bool { return x.ID == p.ID }) {
		return nil
	}
	d.Playlists = append(d.Playlists, p)
	return l.save(d)
}

// Clear は、記録をすべて消す。
func (l *LedgerFile) Clear() error { return l.save(ledgerData{}) }
