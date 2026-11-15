// Package localfile は、手元のファイルを開く。
package localfile

import (
	"fmt"
	"io"
	"os"
)

// Open は、手元のファイルを開き、大きさとともに返す。ディレクトリは開かない。
// ファイルがなければ、os.Open と同じく fs.ErrNotExist を包んだエラーを返す。
func Open(path string) (io.ReadCloser, int64, error) {
	f, err := os.Open(path) //nolint:gosec // 利用者が引数で指定した、上げるファイル
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err == nil && info.IsDir() {
		err = fmt.Errorf("%s はディレクトリです", path)
	}
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}
