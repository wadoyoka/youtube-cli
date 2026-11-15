package a

import "google.golang.org/api/youtube/v3"

func h(svc *youtube.Service) {
	_ = svc.Videos.Delete("id")
	_ = &youtube.VideoStatus{PrivacyStatus: "public"}
}
