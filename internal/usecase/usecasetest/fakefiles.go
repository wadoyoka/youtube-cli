package usecasetest

import (
	"io"
	"io/fs"
	"strings"
)

// Files は、パスごとの中身を持つ偽の手元のファイル。
type Files map[string]string

// Open は、path の中身と大きさを返す。なければ、os.Open と同じく fs.ErrNotExist を包んだエラーを返す。
func (f Files) Open(path string) (io.ReadCloser, int64, error) {
	s, ok := f[path]
	if !ok {
		return nil, 0, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return io.NopCloser(strings.NewReader(s)), int64(len(s)), nil
}
