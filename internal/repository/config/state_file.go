package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/repository/atomicfile"
)

// StateFile は、設定ディレクトリの state.json。ログインしたチャンネルを覚える。
type StateFile struct {
	path string
}

type stateData struct {
	ChannelID         string `json:"channelId"`
	ChannelTitle      string `json:"channelTitle"`
	UploadsPlaylistID string `json:"uploadsPlaylistId"`
}

// NewStateFile は、dir/state.json を扱う StateFile を作る。
func NewStateFile(dir string) *StateFile {
	return &StateFile{path: filepath.Join(dir, "state.json")}
}

// Channel は、保存したチャンネルを返す。ファイルがなければ found=false。
func (s *StateFile) Channel() (domain.Channel, bool, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return domain.Channel{}, false, nil
	}
	if err != nil {
		return domain.Channel{}, false, fmt.Errorf("状態ファイルを読めません: %w", err)
	}
	var st stateData
	if err := json.Unmarshal(data, &st); err != nil {
		return domain.Channel{}, false, fmt.Errorf("状態ファイルが壊れています（%s）: %w", s.path, err)
	}
	if st.ChannelID == "" {
		return domain.Channel{}, false, nil
	}
	return domain.Channel{ID: st.ChannelID, Title: st.ChannelTitle, UploadsPlaylistID: st.UploadsPlaylistID}, true, nil
}

// SetChannel は、チャンネルを保存する。
func (s *StateFile) SetChannel(ch domain.Channel) error {
	data, err := json.Marshal(stateData{ChannelID: ch.ID, ChannelTitle: ch.Title, UploadsPlaylistID: ch.UploadsPlaylistID})
	if err != nil {
		return err
	}
	return atomicfile.Write(s.path, data, 0o600)
}
