// Package usecase は、各コマンドの手順が頼る外部の約束（ポート）と、どの手順も使う YouTube への接続を持つ。
// 手順そのものは、サブパッケージ（account・setup）に置く。
package usecase

import (
	"context"
	"io"

	"github.com/wadoyoka/youtube-cli/internal/domain/policy"
)

// ConfigDir は、設定ディレクトリ。
type ConfigDir interface {
	Path() string
	// Ensure は、なければ作る。作ったときだけ true を返す。
	Ensure() (created bool, err error)
}

// PolicyStore は、拒否リストの置き場。
type PolicyStore interface {
	Path() string
	// CreateIfAbsent は、なければ既定の内容で作る。作ったときだけ true を返す。
	CreateIfAbsent() (created bool, err error)
	Load() (policy.Policy, error)
}

// LoginOptions は、ログインの入出力。
type LoginOptions struct {
	NoBrowser bool
	Prompt    io.Writer // URL と案内を書く先（標準エラー）
	Input     io.Reader // --no-browser のとき、貼り付けを読む元（標準入力）。Login が戻ったあとも読み続けることがある。ほかと共有してよいのは、Login が戻ったあとに誰もその Reader から読まないときだけ（setup は質問役と共有するが、ログインのあとは何も読まない）
}

// AuthInspection は、保存したものが今の鍵で読めるかの結果。
type AuthInspection struct {
	KeyStore     string // "os" か "file"
	ClientSecret bool   // client_secret.enc があり、今の鍵で読める
	Token        bool   // token.enc があり、今の鍵で読める
	Problem      string // 読めない理由。問題がなければ ""
	ProjectID    string // 読めた client_secret.enc の installed.project_id。分からなければ ""。秘密は入れない
}

// Authenticator は、Google へのログインとトークンの置き場。
type Authenticator interface {
	HasClientSecret() bool
	ImportClientSecret(path string) error
	// ClientSecretProjectID は、path の JSON を検めて、installed.project_id を返す（保存はしない）。
	// 読めない・形が違うときは ImportClientSecret と同じ KindUsage。project_id がなければ ""。
	ClientSecretProjectID(path string) (string, error)
	Login(ctx context.Context, opt LoginOptions) error
	Logout(ctx context.Context) error
	HasToken() bool
	Inspect() (AuthInspection, error)
}

// Locker は、ほかの youtube-cli と同時に書き込まないための排他ロック。
type Locker interface {
	Lock(ctx context.Context) (unlock func(), err error)
}

// CloudProject は、gcloud で見たプロジェクト。
type CloudProject struct {
	ID    string
	State string // gcloud の lifecycleState のまま（"ACTIVE"、"DELETE_REQUESTED" など）
}

// CloudSetup は、setup が使う gcloud の操作。作る・有効にするだけで、消す操作は持たない。
type CloudSetup interface {
	Installed() bool
	ActiveAccount(ctx context.Context) (string, error) // ログインしていなければ ""
	DescribeProject(ctx context.Context, id string) (p CloudProject, found bool, err error)
	CreateProject(ctx context.Context, id string) error // ID が使われていれば KindConflict
	EnableYouTube(ctx context.Context, id string) error
}

// SetupPrompter は、setup が人に尋ねるための窓口。cli が実装する。
type SetupPrompter interface {
	Ask(question, def string) (string, error) // Enter だけなら def。入力の終わりは io.EOF
	Confirm(question string) (bool, error)    // y/N。既定は false
	Say(format string, args ...any)           // 案内を1行（標準エラー）
}
