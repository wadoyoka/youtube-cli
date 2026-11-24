// Package setup は、使い始めの準備（init と setup）の手順を持つ。
package setup

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/domain/gcp"
	"github.com/wadoyoka/youtube-cli/internal/domain/winpath"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/account"
)

const (
	enableYouTubeAttempts = 5
	enableYouTubeFirstGap = 2 * time.Second
	projectIDPrefix       = "youtube-cli-"
	suffixAlphabet        = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffixLength          = 6
)

// Setup は、プロジェクトの用意からログインまでを案内しながら進める。
type Setup struct {
	Init         Init
	Policy       usecase.PolicyStore
	Auth         usecase.Authenticator
	Cloud        usecase.CloudSetup
	Login        account.Login
	Status       account.Status
	OpenURL      func(url string) error
	Wait         func(ctx context.Context, d time.Duration) error // nil なら ctx を見ながら time.After で待つ
	RandomSuffix func() (string, error)                           // nil なら crypto/rand で小文字英数字 6 文字
	Windows      bool                                             // true なら、入力欄のパスで \ を外さず、/c/ の形を C:/ に直す
}

// Input は、setup の入力。
type Input struct {
	ProjectID        string
	ClientSecretPath string
	NoBrowser        bool
	Ask              usecase.SetupPrompter
	Prompt           io.Writer // Login に渡す
	Input            io.Reader // Login に渡す。Ask と同じ読み手であること
}

// Result は、setup の結果。
type Result struct {
	ProjectID     string
	GCloudAccount string
	ChannelTitle  string
	ChannelURL    string
	KeyStore      string
}

const errSetupAborted = "中止しました"

// Aborted は、setup を人が止めた（入力の終わり、空の入力、Ctrl-C）ときのエラー。終了コード 2。
func Aborted() error { return domain.Errorf(domain.KindUsage, "%s", errSetupAborted) }

func setupAborted() error { return Aborted() }

// Run は、設計書 4.2 の手順 1〜12 を順に行う。済んでいる手順は飛ばす。
// 途中で ctx が取り消された（Ctrl-C）ときは、gcloud などの失敗ではなく「中止しました」を返す。
func (u Setup) Run(ctx context.Context, in Input) (Result, error) {
	res, err := u.run(ctx, in)
	if err != nil && ctx.Err() != nil {
		return Result{}, setupAborted()
	}
	return res, err
}

func (u Setup) run(ctx context.Context, in Input) (Result, error) {
	say := in.Ask.Say

	// 1. init と同じ処理
	if _, err := u.Init.Run(); err != nil {
		return Result{}, err
	}
	say("[済み] 設定ディレクトリと policy.toml を用意しました")

	// 2. 拒否リスト
	pol, err := u.Policy.Load()
	if err != nil {
		return Result{}, err
	}
	for _, name := range []string{"setup", "login"} {
		if err := pol.Check(name); err != nil {
			return Result{}, err
		}
	}

	// 3. 鍵の置き場所（GCP を変える前に確かめる）
	insp, err := u.Auth.Inspect()
	if err != nil {
		return Result{}, err
	}

	// --project の形は、gcloud を呼ぶ前に確かめる（引数の誤りは終了コード 2）。
	if in.ProjectID != "" {
		if err := gcp.ValidateProjectID(in.ProjectID); err != nil {
			return Result{}, err
		}
	}
	// --client-secret も、gcloud を呼ぶ前に検める（読むだけで、保存はしない）。project_id は ID の既定値に使う。
	secretProjectID := ""
	if in.ClientSecretPath != "" {
		if secretProjectID, err = u.Auth.ClientSecretProjectID(in.ClientSecretPath); err != nil {
			return Result{}, err
		}
	}

	// 4. gcloud
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if !u.Cloud.Installed() {
		return Result{}, domain.Errorf(domain.KindSetup, "gcloud が見つかりません。Google Cloud CLI を入れてから、やり直してください: https://cloud.google.com/sdk/docs/install")
	}

	// 5. gcloud のアカウント
	gcloudAccount, err := u.Cloud.ActiveAccount(ctx)
	if err != nil {
		return Result{}, err
	}
	if gcloudAccount == "" {
		return Result{}, domain.Errorf(domain.KindSetup, "`gcloud auth login` を実行してから、やり直してください")
	}
	say("[確認] gcloud のアカウント %s でプロジェクトを作ります", gcloudAccount)

	// 6, 7. プロジェクト
	id, err := u.resolveProject(ctx, in, defaultProjectID(secretProjectID, insp))
	if err != nil {
		return Result{}, err
	}
	say("やり直すときは `youtube-cli setup --project %s` を使ってください", id)

	// 8. YouTube Data API
	if err := u.enableYouTube(ctx, id); err != nil {
		return Result{}, err
	}
	say("[済み] YouTube Data API を有効にしました")

	// 9, 10. クライアントシークレット
	imported := false
	if in.ClientSecretPath != "" || !insp.ClientSecret {
		if in.ClientSecretPath == "" {
			u.guide(id, in)
		}
		if err := u.importClientSecret(id, in); err != nil {
			return Result{}, err
		}
		imported = true
		say("[済み] クライアントシークレットを取り込みました。JSON は安全な場所に保管してください")
	} else {
		say("[省略] クライアントシークレットは取り込み済みです")
	}

	// 11, 12. ログイン
	res := Result{ProjectID: id, GCloudAccount: gcloudAccount, KeyStore: insp.KeyStore}
	if !imported && insp.Token {
		st, err := u.Status.Run(ctx)
		switch {
		case err == nil:
			say("[省略] ログイン済みです")
			res.ChannelTitle, res.ChannelURL = st.ChannelTitle, st.ChannelURL
			return res, nil
		case domain.KindOf(err) == domain.KindSetup && ctx.Err() == nil:
			// token.enc は読めるが、Google 側で取り消された・期限が切れたなど。ログインし直す。
			// 原因（通信の応答など）までは出さず、こちらで書いた文言だけを出す。
			if de, ok := errors.AsType[*domain.Error](err); ok {
				say("[確認] 保存したログインが使えません（%s）。ログインし直します", de.Message)
			}
		default:
			return Result{}, err
		}
	}
	lr, err := u.Login.Run(ctx, account.LoginInput{NoBrowser: in.NoBrowser, Prompt: in.Prompt, Input: in.Input})
	if err != nil {
		return Result{}, err
	}
	say("[済み] チャンネル %s でログインしました", lr.ChannelTitle)
	res.ChannelTitle, res.ChannelURL = lr.ChannelTitle, lr.ChannelURL
	return res, nil
}

// defaultProjectID は、--project がないときに示す既定値の候補。
// --client-secret の project_id、取り込み済みの client_secret.enc の project_id の順に使い、
// どちらもなければ ""（そのときはランダムな ID にする）。済んだ setup をやり直しても、新しいプロジェクトを作らないため。
func defaultProjectID(secretProjectID string, insp usecase.AuthInspection) string {
	if secretProjectID != "" {
		return secretProjectID
	}
	if insp.ClientSecret {
		return insp.ProjectID
	}
	return ""
}

// resolveProject は、手順 6 と 7。使うプロジェクトの ID を返す。def は対話で示す既定値（"" ならランダム）。
func (u Setup) resolveProject(ctx context.Context, in Input, def string) (string, error) {
	say := in.Ask.Say
	interactive := in.ProjectID == ""
	id := in.ProjectID
	if !interactive {
		if err := gcp.ValidateProjectID(id); err != nil {
			return "", err
		}
	}
	for {
		if interactive {
			var err error
			if id, err = u.askProjectID(in.Ask, def); err != nil {
				return "", err
			}
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		p, found, err := u.Cloud.DescribeProject(ctx, id)
		if err != nil {
			return "", err
		}
		if found {
			switch p.State {
			case "ACTIVE":
				say("[済み] プロジェクト %s は既にあります", id)
				return id, nil
			case "DELETE_REQUESTED":
				return "", domain.Errorf(domain.KindSetup, "プロジェクト %s は削除予定です。`gcloud projects undelete %s` で戻してから、やり直してください", id, id)
			default:
				return "", domain.Errorf(domain.KindSetup, "プロジェクト %s の状態が %s です", id, p.State)
			}
		}
		err = u.Cloud.CreateProject(ctx, id)
		if err == nil {
			say("[作成] プロジェクト %s を作りました", id)
			return id, nil
		}
		if domain.KindOf(err) == domain.KindConflict && interactive {
			say("プロジェクト ID %s は他の人に使われています。別の ID にしてください", id)
			continue
		}
		return "", err
	}
}

// askProjectID は、形の合う ID が得られるまで聞く。def が "" なら、ランダムな ID を既定値にする。
func (u Setup) askProjectID(ask usecase.SetupPrompter, def string) (string, error) {
	if def == "" {
		suffix, err := u.randomSuffix()
		if err != nil {
			return "", err
		}
		def = projectIDPrefix + suffix
	}
	for {
		ans, err := ask.Ask("プロジェクト ID", def)
		if errors.Is(err, io.EOF) {
			return "", setupAborted()
		}
		if err != nil {
			return "", err
		}
		ans = strings.TrimSpace(ans)
		if ans == "" {
			ans = def
		}
		if err := gcp.ValidateProjectID(ans); err != nil {
			ask.Say("%s", err.Error())
			continue
		}
		return ans, nil
	}
}

func (u Setup) randomSuffix() (string, error) {
	if u.RandomSuffix != nil {
		return u.RandomSuffix()
	}
	b := make([]byte, suffixLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(suffixAlphabet))))
		if err != nil {
			return "", domain.Errorf(domain.KindUnexpected, "乱数を作れません").Wrap(err)
		}
		b[i] = suffixAlphabet[n.Int64()]
	}
	return string(b), nil
}

// enableYouTube は、手順 8。作った直後は失敗しうるので、2、4、8、16 秒あけて最大 5 回試す。
func (u Setup) enableYouTube(ctx context.Context, id string) error {
	gap := enableYouTubeFirstGap
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := u.Cloud.EnableYouTube(ctx, id)
		if err == nil {
			return nil
		}
		if attempt == enableYouTubeAttempts {
			return err
		}
		if err := u.wait(ctx, gap); err != nil {
			return err
		}
		gap *= 2
	}
}

func (u Setup) wait(ctx context.Context, d time.Duration) error {
	if u.Wait != nil {
		return u.Wait(ctx, d)
	}
	return waitCtx(ctx, d)
}

func waitCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// guide は、手順 9。Google Auth Platform での手作業を案内し、概要の画面を開く。
func (u Setup) guide(id string, in Input) {
	say := in.Ask.Say
	say("次の4つを、ブラウザで行ってください（クライアントの JSON は、作成直後の画面でしかダウンロードできません）")
	say("  1. ブランディング %s : アプリ名とサポート用メールアドレスを設定する", gcp.AuthConsoleURL("branding", id))
	say("  2. 対象 %s : ユーザーの種類を「外部」にする。公開ステータスは「テスト」のままにする。「テストユーザー」に、この CLI で YouTube を使う Google アカウント（自分を含む）を追加する（テストユーザーでないアカウントはログインできません。リフレッシュトークンは 7 日で切れるので、そのたびに youtube-cli login をやり直します）", gcp.AuthConsoleURL("audience", id))
	say("  3. データアクセス %s : https://www.googleapis.com/auth/youtube.force-ssl を追加する", gcp.AuthConsoleURL("scopes", id))
	say("  4. クライアント %s : 種類「デスクトップアプリ」のクライアントを作り、作成直後の画面で JSON をダウンロードする。この画面を閉じるとシークレットは二度と表示されません。JSON はパスワードマネージャーなど安全な場所に保管してください", gcp.AuthConsoleURL("clients", id))
	if in.NoBrowser || u.OpenURL == nil {
		return
	}
	if err := u.OpenURL(gcp.AuthConsoleURL("overview", id)); err != nil {
		say("ブラウザを開けませんでした。上の URL を開いてください")
	}
}

// importClientSecret は、手順 10。JSON のパスを決めて取り込む。
func (u Setup) importClientSecret(id string, in Input) error {
	say := in.Ask.Say
	explicit := in.ClientSecretPath != ""
	for {
		path := in.ClientSecretPath
		if !explicit {
			ans, err := in.Ask.Ask("ダウンロードした JSON のパス（ファイルをここへドラッグしてもよい）", "")
			if errors.Is(err, io.EOF) {
				return setupAborted()
			}
			if err != nil {
				return err
			}
			if path, err = normalizeInputPath(ans, u.Windows); err != nil {
				say("%s", err.Error())
				continue
			}
			if path == "" {
				return setupAborted()
			}
		}
		pid, err := u.Auth.ClientSecretProjectID(path)
		if err != nil {
			if domain.KindOf(err) != domain.KindUsage || explicit {
				return err
			}
			say("%s", err.Error())
			continue
		}
		if pid != "" && pid != id {
			say("警告: この JSON のプロジェクトは %s で、setup のプロジェクト %s と違います", pid, id)
			ok, err := in.Ask.Confirm("このまま使いますか")
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			if err != nil || !ok {
				if explicit {
					return setupAborted()
				}
				continue
			}
		}
		return u.Auth.ImportClientSecret(path)
	}
}

// normalizeInputPath は、人が打った（またはドラッグした）パスを整える。
// 前後の空白と両端の ' か " を1組取り除く。引用符で囲まれていなければ、`\` の後ろの1文字をそのまま残す
// （`x\ \(1\).json` → `x (1).json`）。先頭の `~/` はホームに展開する。
// windows のときは、`\` を外さず、`/c/...` の形を `C:/...` に直し、`~\` も `~/` と同じくホームに展開する。
func normalizeInputPath(s string, windows bool) (string, error) {
	s = strings.TrimSpace(s)
	quoted := false
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		s = s[1 : len(s)-1]
		quoted = true
	}
	rest, tilde := strings.CutPrefix(s, "~/")
	if !tilde && windows {
		rest, tilde = strings.CutPrefix(s, `~\`)
	}
	if windows {
		if !tilde {
			rest = winpath.FromMSYS(rest)
		}
	} else if !quoted {
		rest = unescapeBackslashes(rest)
	}
	if !tilde {
		return rest, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", domain.Errorf(domain.KindUsage, "ホームディレクトリを調べられません: %s", s).Wrap(err)
	}
	return filepath.Join(home, rest), nil
}

// unescapeBackslashes は、`\` とその後ろの1文字を、その1文字にする。末尾に残った `\` はそのまま。
func unescapeBackslashes(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
