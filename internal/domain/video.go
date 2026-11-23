package domain

import "time"

// Video は、YouTube の動画。
type Video struct {
	ID            string    `json:"videoId"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Tags          []string  `json:"tags"`
	PrivacyStatus string    `json:"privacyStatus"`
	UploadStatus  string    `json:"uploadStatus"`
	PublishedAt   time.Time `json:"publishedAt"`
}

// URL は、動画の短い URL を返す。
func (v Video) URL() string { return "https://youtu.be/" + v.ID }

// StudioURL は、YouTube Studio の編集ページの URL を返す。
func (v Video) StudioURL() string { return "https://studio.youtube.com/video/" + v.ID + "/edit" }
