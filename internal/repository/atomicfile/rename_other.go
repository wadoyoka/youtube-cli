//go:build !windows

package atomicfile

import "os"

// replaceFile は、newpath を oldpath で置き換える。
func replaceFile(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
