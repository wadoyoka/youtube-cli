package domain

// Playlist は、YouTube の再生リスト。
type Playlist struct {
	ID            string `json:"playlistId"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	PrivacyStatus string `json:"privacyStatus"`
	ItemCount     int64  `json:"itemCount"`
}

// URL は、再生リストのページの URL を返す。
func (p Playlist) URL() string { return "https://www.youtube.com/playlist?list=" + p.ID }
