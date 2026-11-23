package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

const lockPollInterval = 100 * time.Millisecond

// LockFile は、設定ディレクトリの lock。OS のファイルロックで、ほかの youtube-cli と排他にする。
type LockFile struct {
	path string
}

// NewLockFile は、dir/lock を扱う LockFile を作る。
func NewLockFile(dir string) *LockFile {
	return &LockFile{path: filepath.Join(dir, "lock")}
}

// Lock は、排他ロックを取る。取れない間は 100ms ごとに試し、ctx が終わったらエラーにする。
func (l *LockFile) Lock(ctx context.Context) (func(), error) {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("ロックファイルを開けません: %w", err)
	}
	for {
		locked, err := tryLock(f)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("ロックを取れません: %w", err)
		}
		if locked {
			return func() {
				unlock(f)
				_ = f.Close()
			}, nil
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, domain.Errorf(domain.KindUnexpected, "ほかの youtube-cli がロックを持ったままです").Wrap(ctx.Err())
		case <-time.After(lockPollInterval):
		}
	}
}
