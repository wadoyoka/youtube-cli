package atomicfile

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

const (
	renameAttempts = 20
	renameWait     = 50 * time.Millisecond
)

// replaceFile は、newpath を oldpath で置き換える。
// Windows では、ほかのプロセスが newpath を開いている間は置き換えに失敗することがあるため、
// そのときだけ合計で約 1 秒、試し直す。
func replaceFile(oldpath, newpath string) error {
	return renameRetrying(os.Rename, isSharingError, oldpath, newpath, renameAttempts, renameWait)
}

func isSharingError(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
