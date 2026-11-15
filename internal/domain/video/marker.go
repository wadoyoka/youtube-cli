package video

import "strings"

// PlaylistMarker は、このツールが作った再生リストの説明の最後に入れる目印。
const PlaylistMarker = "[youtube-cli]"

// AddPlaylistMarker は、説明の最後に目印を足す。
func AddPlaylistMarker(desc string) string {
	if desc == "" {
		return PlaylistMarker
	}
	return desc + "\n\n" + PlaylistMarker
}

// HasPlaylistMarker は、説明の最後の空でない行が目印かを返す。
func HasPlaylistMarker(desc string) bool {
	lines := strings.Split(strings.TrimRight(desc, " \t\r\n"), "\n")
	return strings.TrimSpace(lines[len(lines)-1]) == PlaylistMarker
}

// Unusable は、アップロード状態が使えない動画（失敗・却下・削除）かを返す。
func Unusable(uploadStatus string) bool {
	switch uploadStatus {
	case "failed", "rejected", "deleted":
		return true
	}
	return false
}
