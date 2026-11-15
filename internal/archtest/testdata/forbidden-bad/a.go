package a

import (
	"google.golang.org/api/youtube/v3"
	yt "google.golang.org/api/youtube/v3"
)

func f(svc *youtube.Service) {
	_ = svc.Videos.Delete("id")
	_ = svc.Videos.Update([]string{"snippet"}, nil)
	_ = svc.Playlists.Delete("id")
	_ = svc.Playlists.Update([]string{"snippet"}, nil)
	_ = svc.PlaylistItems.Delete("id")
	_ = svc.PlaylistItems.Update([]string{"snippet"}, nil)
	_ = svc.Comments
	_ = svc.CommentThreads
	_ = svc.Captions
	_ = svc.ChannelBanners
	_ = svc.Channels.Update([]string{"snippet"}, nil)
	_ = svc.Watermarks
	_ = svc.LiveBroadcasts
	_ = svc.LiveStreams
}

func g(v *youtube.Video, other string) {
	_ = &youtube.VideoStatus{PrivacyStatus: "public"}
	_ = youtube.PlaylistStatus{PrivacyStatus: other}
	_ = &yt.VideoStatus{PrivacyStatus: "private"}
	st := &youtube.VideoStatus{}
	st.PrivacyStatus = "private"
	v.Status.PrivacyStatus = "public"
	var ps yt.PlaylistStatus
	ps.PrivacyStatus = other
}
