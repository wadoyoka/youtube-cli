package auth

import (
	"errors"
	"io/fs"
	"os"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/repository/atomicfile"
)

// SnapshotToken は、今の token.enc の中身（暗号化されたまま）を覚える。
// 返す restore は、その中身を書き戻す。token.enc がなかったときは、token.enc を消す。
func (a *Auth) SnapshotToken() (restore func() error, err error) {
	path := a.tokenPath()
	data, err := os.ReadFile(path) //nolint:gosec // 設定ディレクトリの token.enc
	if errors.Is(err, fs.ErrNotExist) {
		return func() error {
			if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
				return domain.Errorf(domain.KindUnexpected, "%s を消せません", tokenName).Wrap(rmErr)
			}
			return nil
		}, nil
	}
	if err != nil {
		return nil, domain.Errorf(domain.KindUnexpected, "%s を読めません", tokenName).Wrap(err)
	}
	return func() error {
		if err := atomicfile.Write(path, data, 0o600); err != nil {
			return domain.Errorf(domain.KindUnexpected, "%s を元に戻せません", tokenName).Wrap(err)
		}
		return nil
	}, nil
}
