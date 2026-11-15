package usecasetest

import (
	"context"

	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// Auth は、呼ばれた順を覚える偽の Authenticator。
type Auth struct {
	SecretSaved bool
	TokenSaved  bool
	Calls       []string
	Opt         usecase.LoginOptions
	Imported    string
	KeyStore    string
	Problem     string
	InspErr     error
	SnapshotErr error  // SnapshotToken が返すエラー
	Snapshots   int    // SnapshotToken を呼んだ回数
	ProjectID   string // Inspect が返す、取り込み済みの client_secret の project_id

	// ClientSecretProjectID の偽物。
	SecretProjects map[string]string // path -> project_id
	SecretErrs     map[string]error  // path -> 検めの失敗
	Checked        []string
}

// ClientSecretProjectID は、SecretErrs に path があればそのエラーを、なければ SecretProjects[path] を返す。
func (a *Auth) ClientSecretProjectID(path string) (string, error) {
	a.Checked = append(a.Checked, path)
	if err := a.SecretErrs[path]; err != nil {
		return "", err
	}
	return a.SecretProjects[path], nil
}

// HasClientSecret は、取り込み済みかを返す。
func (a *Auth) HasClientSecret() bool { return a.SecretSaved }

// ImportClientSecret は、取り込んだことにする。
func (a *Auth) ImportClientSecret(path string) error {
	a.Calls = append(a.Calls, "import")
	a.Imported = path
	a.SecretSaved = true
	return nil
}

// Login は、ログインしたことにする。
func (a *Auth) Login(_ context.Context, opt usecase.LoginOptions) error {
	a.Calls = append(a.Calls, "login")
	a.Opt = opt
	a.TokenSaved = true
	return nil
}

// Logout は、ログアウトしたことにする。
func (a *Auth) Logout(context.Context) error {
	a.Calls = append(a.Calls, "logout")
	a.TokenSaved = false
	return nil
}

// HasToken は、ログイン済みかを返す。
func (a *Auth) HasToken() bool { return a.TokenSaved }

// Inspect は、持っている値から検めの結果を作る。
func (a *Auth) Inspect() (usecase.AuthInspection, error) {
	insp := usecase.AuthInspection{KeyStore: a.KeyStore, ClientSecret: a.SecretSaved, Token: a.TokenSaved, Problem: a.Problem}
	if a.SecretSaved {
		insp.ProjectID = a.ProjectID
	}
	return insp, a.InspErr
}

// SnapshotToken は、Snapshots を数え、呼ぶと "restore" を Calls に記録して TokenSaved を戻す restore を返す。
func (a *Auth) SnapshotToken() (func() error, error) {
	a.Snapshots++
	if a.SnapshotErr != nil {
		return nil, a.SnapshotErr
	}
	before := a.TokenSaved
	return func() error {
		a.Calls = append(a.Calls, "restore")
		a.TokenSaved = before
		return nil
	}, nil
}
