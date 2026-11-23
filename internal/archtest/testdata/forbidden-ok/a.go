package a

import (
	"google.golang.org/api/youtube/v3"

	"github.com/wadoyoka/youtube-cli/internal/domain/video"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
)

type (
	// listItem は cli の出力の構造体。値を YouTube へ送るものではない。
	listItem struct {
		ID            string
		PrivacyStatus string
	}
	domainVideo struct{ PrivacyStatus string }
)

func f(svc *youtube.Service, s *youtube.VideoStatus, d *domainVideo) {
	_ = svc.Videos.List([]string{"snippet"})
	_ = svc.Videos.Insert([]string{"status"}, nil)
	_ = svc.Playlists.Insert([]string{"status"}, nil)
	_ = svc.PlaylistItems.Insert([]string{"snippet"}, nil)
	_ = &youtube.VideoStatus{PrivacyStatus: video.PrivacyUnlisted}
	_ = &youtube.PlaylistStatus{PrivacyStatus: dv.PrivacyUnlisted}
	st := &youtube.VideoStatus{}
	st.PrivacyStatus = video.PrivacyUnlisted
	// 読み取り（API の応答を domain や cli の構造体へ写す）は対象外。
	d.PrivacyStatus = s.PrivacyStatus
	_ = listItem{ID: "x", PrivacyStatus: d.PrivacyStatus}
}
