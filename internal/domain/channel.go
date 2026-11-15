package domain

// Channel は、ログインした YouTube のチャンネル。
type Channel struct {
	ID                string `json:"channelId"`
	Title             string `json:"channelTitle"`
	UploadsPlaylistID string `json:"uploadsPlaylistId"`
}

// URL は、チャンネルのページの URL を返す。
func (c Channel) URL() string { return "https://www.youtube.com/channel/" + c.ID }
