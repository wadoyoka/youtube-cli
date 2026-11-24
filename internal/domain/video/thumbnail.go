package video

import (
	"path/filepath"
	"strings"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

// maxThumbnailBytes は、サムネイルの大きさの上限（2 MiB）。
const maxThumbnailBytes = 2 * 1024 * 1024

// ValidateThumbnail は、サムネイルの形式と大きさを確かめ、MIME タイプを返す。
func ValidateThumbnail(name string, size int64) (string, error) {
	var mime string
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".png":
		mime = "image/png"
	default:
		return "", domain.Errorf(domain.KindUsage, "サムネイルは jpg か png にしてください: %s", name)
	}
	if size > maxThumbnailBytes {
		return "", domain.Errorf(domain.KindUsage, "サムネイルは 2 MiB 以内にしてください")
	}
	return mime, nil
}
