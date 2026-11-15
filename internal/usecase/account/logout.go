package account

import (
	"context"

	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// Logout は、保存したトークンを消す。
type Logout struct{ Auth usecase.Authenticator }

// Run は、ログアウトする。
func (u Logout) Run(ctx context.Context) error { return u.Auth.Logout(ctx) }
