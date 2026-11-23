// Package auth は、Google へのログイン（OAuth）と、暗号化して保存するトークン・クライアントシークレット、その鍵の置き場所を持つ。
package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/youtube/v3"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/repository/atomicfile"
	"github.com/wadoyoka/youtube-cli/internal/repository/browser"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

const (
	clientSecretName       = "client_secret.enc"
	tokenName              = "token.enc"
	legacyClientSecretName = "client_secret.json"
	legacyTokenName        = "token.json"
	defaultRevokeURL       = "https://oauth2.googleapis.com/revoke"
	defaultTimeout         = 5 * time.Minute
	keyLockTimeout         = 30 * time.Second
)

const (
	msgNoKey = "鍵が見つかりません。`youtube-cli login --client-secret <JSON>` をやり直してください"
	// msgNoKeyFileHint は、OS のキーチェーンに鍵がないのに、設定ディレクトリに key があるときの文言。
	msgNoKeyFileHint = "鍵が見つかりません。設定ディレクトリに key があります。`YOUTUBE_CLI_KEYRING=file` を付け忘れていませんか。付け忘れでなければ、`youtube-cli login --client-secret <JSON>` をやり直してください"
	msgMismatch      = "今の鍵で読めません。`youtube-cli login --client-secret <JSON>` をやり直してください"
	msgLegacy        = "古い形式のファイルがあります。`youtube-cli login --client-secret <JSON>` をやり直してください"
	msgNotDesktop    = "client_secret の形式が正しくありません（デスクトップアプリの OAuth クライアントの JSON を指定してください）"
	msgWebClient     = "client_secret の種類が違います（ウェブアプリケーションなどのクライアントです）。デスクトップアプリの OAuth クライアントの JSON を指定してください"
)

// Auth は、Google へのログインと、設定ディレクトリのトークンを扱う。
type Auth struct {
	Dir  string
	Keys KeyStore
	Lock func(ctx context.Context) (unlock func(), err error) // 鍵を作る・移すときの排他。nil ならロックしない

	mu  sync.Mutex
	key []byte // Keys.Load・LoadLocked で見つけた鍵。mu で守る

	OpenBrowser  func(url string) error
	Endpoint     oauth2.Endpoint // ゼロ値なら google.Endpoint
	RevokeURL    string          // "" なら https://oauth2.googleapis.com/revoke
	HTTPClient   *http.Client    // nil なら http.DefaultClient
	LoginTimeout time.Duration   // 0 なら 5 分
}

// NewAuth は、dir を使う Auth を返す。
func NewAuth(dir string, keys KeyStore, lock func(context.Context) (func(), error)) *Auth {
	return &Auth{Dir: dir, Keys: keys, Lock: lock, OpenBrowser: browser.Open}
}

func (a *Auth) clientSecretPath() string { return filepath.Join(a.Dir, clientSecretName) }
func (a *Auth) tokenPath() string        { return filepath.Join(a.Dir, tokenName) }

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// HasClientSecret は、client_secret.enc があるかを返す。
func (a *Auth) HasClientSecret() bool { return fileExists(a.clientSecretPath()) }

// HasToken は、token.enc があるかを返す。
func (a *Auth) HasToken() bool { return fileExists(a.tokenPath()) }

func (a *Auth) httpClient() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return http.DefaultClient
}

func (a *Auth) withHTTPClient(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, a.httpClient())
}

// loadKey は、鍵を Keys.Load で探す。見つけたらメモリに持つ。鍵を作ることはしない。
// Unverified の鍵は、adoptKey で今のファイルを復号できるかを確かめてから使う。
// 鍵が Unverified のときは adoptKey が設定ディレクトリのロックを取る。TokenSource が Remote.Locker の区間の前に鍵を読んで持つので今は安全だが、
// 区間の中で初めて呼ぶと keyLockTimeout まで待って失敗する。
func (a *Auth) loadKey() (key []byte, found bool, err error) {
	loaded, found, err := a.cachedOrLoadKey()
	if err != nil || !found {
		return nil, false, err
	}
	if loaded.Unverified {
		// mu は放してある。adoptKey は Lock → mu の順で取る（ensureKey と同じ順。逆にするとプロセスの中で止まりうる）。
		return a.adoptKey(context.Background())
	}
	return loaded.Key, true, nil
}

// cachedOrLoadKey は、mu の中で、メモリの鍵か Keys.Load の結果を返す。メモリに持つのは Unverified でない鍵だけ。
// Keys.Load はロックの外で呼ぶので、Unverified の結果は鍵を持たない合図として扱い、その Key は使わない（adoptKey が読み直す）。
func (a *Auth) cachedOrLoadKey() (LoadedKey, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.key != nil {
		return LoadedKey{Key: a.key}, true, nil
	}
	loaded, found, err := a.Keys.Load()
	if err != nil || !found {
		return LoadedKey{}, false, err
	}
	if !loaded.Unverified {
		a.key = loaded.Key
	}
	return loaded, true, nil
}

// adoptKey は、Lock の中で鍵を Keys.LoadLocked で読み直す。Unverified の鍵は、今のファイルを復号できるときだけ Adopt で新しい形に移して、メモリに持つ。
// ファイルがない、または復号できないときは、移さず、メモリにも持たずにその鍵を返す（読む側は msgMismatch で止まる）。
func (a *Auth) adoptKey(ctx context.Context) (key []byte, found bool, err error) {
	unlock, err := a.lockKeys(ctx)
	if err != nil {
		return nil, false, err
	}
	defer unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.key != nil { // ロックを待つ間に、同じプロセスのほかの呼び出しが確かめ終えた
		return a.key, true, nil
	}
	loaded, found, err := a.Keys.LoadLocked()
	if err != nil || !found {
		return nil, false, err
	}
	if loaded.Unverified {
		fit, err := a.checkKey(loaded.Key)
		if err != nil {
			return nil, false, err
		}
		if fit != keyFits {
			return loaded.Key, true, nil
		}
		if err := a.Keys.Adopt(loaded.Key); err != nil {
			return nil, false, err
		}
	}
	// Unverified でなければ、ロックを待つ間にほかの youtube-cli が移し終えている。Adopt は呼ばない。
	a.key = loaded.Key
	return loaded.Key, true, nil
}

// lockKeys は、鍵を作る・移すための Lock を、keyLockTimeout まで待って取る。Lock が nil なら何もしない。
func (a *Auth) lockKeys(ctx context.Context) (unlock func(), err error) {
	if a.Lock == nil {
		return func() {}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, keyLockTimeout)
	defer cancel()
	unlock, err = a.Lock(ctx)
	if err != nil {
		return nil, domain.Errorf(domain.KindUnexpected, "鍵を作る順番を待てません").Wrap(err)
	}
	return unlock, nil
}

// keyFit は、鍵で今のファイルを復号できるか。
type keyFit int

const (
	keyNoFiles  keyFit = iota // client_secret.enc も token.enc もない
	keyFits                   // あるものがすべて、この鍵で復号できる
	keyMismatch               // 復号できないものがある
)

// checkKey は、client_secret.enc と token.enc のうちあるものを、key で復号できるかを調べる。
// 読めない（権限など）ときは KindUnexpected。
func (a *Auth) checkKey(key []byte) (keyFit, error) {
	fit := keyNoFiles
	for _, f := range []struct{ name, kind string }{
		{clientSecretName, secretKindClientSecret},
		{tokenName, secretKindToken},
	} {
		sealed, err := os.ReadFile(filepath.Join(a.Dir, f.name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return keyNoFiles, domain.Errorf(domain.KindUnexpected, "%s を読めません", f.name).Wrap(err)
		}
		if _, err := openSecret(key, f.kind, sealed); err != nil {
			return keyMismatch, nil
		}
		fit = keyFits
	}
	return fit, nil
}

// ensureKey は、Lock の中で鍵を Keys.LoadLocked で探し、今のファイルを復号できる鍵ならそれを使う（Unverified なら Adopt で移す）。
// 見つからない、またはファイルがない・復号できない鍵なら、新しい鍵を作って置き換える。新しく作ったら created が true。
// Lock はもう持っているので、adoptKey は呼ばない（Lock は入れ子にできない）。
func (a *Auth) ensureKey(ctx context.Context) (key []byte, created bool, err error) {
	unlock, err := a.lockKeys(ctx)
	if err != nil {
		return nil, false, err
	}
	defer unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	loaded, found, err := a.Keys.LoadLocked()
	if err != nil {
		return nil, false, err
	}
	if found {
		fit, err := a.checkKey(loaded.Key)
		if err != nil {
			return nil, false, err
		}
		if fit == keyFits {
			if loaded.Unverified {
				if err := a.Keys.Adopt(loaded.Key); err != nil {
					return nil, false, err
				}
			}
			a.key = loaded.Key
			return loaded.Key, false, nil
		}
		// ファイルがない、または復号できない鍵は、youtube-cli が作った鍵だと言えないので使わない（設計書 5.7）。
	}
	key, err = newKey()
	if err != nil {
		return nil, false, domain.Errorf(domain.KindUnexpected, "鍵を作れません").Wrap(err)
	}
	if err := a.Keys.Save(key); err != nil {
		return nil, false, err
	}
	a.key = key
	return key, true, nil
}

// readSealed は、name を読んで今の鍵で復号する。ファイルがなければ os.ErrNotExist を返す。
func (a *Auth) readSealed(name, kind string) ([]byte, error) {
	sealed, err := os.ReadFile(filepath.Join(a.Dir, name)) //nolint:gosec // 設定ディレクトリの中
	if errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err != nil {
		return nil, domain.Errorf(domain.KindUnexpected, "%s を読めません", name).Wrap(err)
	}
	key, found, err := a.loadKey()
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, domain.Errorf(domain.KindSetup, "%s", a.noKeyMessage())
	}
	plain, err := openSecret(key, kind, sealed)
	if err != nil {
		return nil, domain.Errorf(domain.KindSetup, "%s", msgMismatch).Wrap(err)
	}
	return plain, nil
}

// noKeyMessage は、鍵が見つからないときの文言。OS のキーチェーンを見ていて、設定ディレクトリに key があれば、
// YOUTUBE_CLI_KEYRING=file の付け忘れを疑う一言を足す（付け忘れたまま取り込み直すと、新しい鍵で上書きしてしまうため）。
func (a *Auth) noKeyMessage() string {
	if a.Keys.Kind() == "os" && fileExists(filepath.Join(a.Dir, keyFileName)) {
		return msgNoKeyFileHint
	}
	return msgNoKey
}

func (a *Auth) writeSealed(name, kind string, plain []byte) error {
	key, found, err := a.loadKey()
	if err != nil {
		return err
	}
	if !found {
		return domain.Errorf(domain.KindSetup, "鍵が見つかりません。`youtube-cli login` をやり直してください")
	}
	return a.writeSealedWith(key, name, kind, plain)
}

func (a *Auth) writeSealedWith(key []byte, name, kind string, plain []byte) error {
	sealed, err := sealSecret(key, kind, plain)
	if err != nil {
		return domain.Errorf(domain.KindUnexpected, "%s を暗号化できません", name).Wrap(err)
	}
	if err := atomicfile.Write(filepath.Join(a.Dir, name), sealed, 0o600); err != nil {
		return domain.Errorf(domain.KindUnexpected, "%s を保存できません", name).Wrap(err)
	}
	return nil
}

func (a *Auth) legacyExists() bool {
	return fileExists(filepath.Join(a.Dir, legacyClientSecretName)) || fileExists(filepath.Join(a.Dir, legacyTokenName))
}

func (a *Auth) config() (*oauth2.Config, error) {
	data, err := a.readSealed(clientSecretName, secretKindClientSecret)
	if errors.Is(err, os.ErrNotExist) {
		if a.legacyExists() {
			return nil, domain.Errorf(domain.KindSetup, "%s", msgLegacy)
		}
		return nil, domain.Errorf(domain.KindSetup, "クライアントシークレットがありません。`youtube-cli login --client-secret <path>` で取り込んでください")
	}
	if err != nil {
		return nil, err
	}
	cfg, err := google.ConfigFromJSON(data, youtube.YoutubeForceSslScope)
	if err != nil {
		return nil, domain.Errorf(domain.KindSetup, "クライアントシークレットを読めません。`youtube-cli login --client-secret <path>` で取り込み直してください").Wrap(err)
	}
	if a.Endpoint != (oauth2.Endpoint{}) {
		cfg.Endpoint = a.Endpoint
	} else {
		cfg.Endpoint = google.Endpoint
	}
	return cfg, nil
}

// readClientSecretFile は、path の client_secret を読み、形を検める。中身と installed.project_id を返す。
func readClientSecretFile(path string) (data []byte, projectID string, err error) {
	data, err = os.ReadFile(path) //nolint:gosec // 利用者が指定したパスを読むのが目的
	if err != nil {
		return nil, "", domain.Errorf(domain.KindUsage, "client_secret のファイルを読めません: %s", path).Wrap(err)
	}
	projectID, err = parseClientSecret(data)
	if err != nil {
		return nil, "", err
	}
	return data, projectID, nil
}

// parseClientSecret は、client_secret の JSON がデスクトップアプリの OAuth クライアントのものかを検め、
// installed.project_id を返す。ConfigFromJSON は web も受け付けるが、web のクライアントでは
// ループバックへのリダイレクトが通らないので、installed がない、または web があれば KindUsage で断る。
func parseClientSecret(data []byte) (projectID string, err error) {
	if _, err := google.ConfigFromJSON(data, youtube.YoutubeForceSslScope); err != nil {
		return "", domain.Errorf(domain.KindUsage, "%s", msgNotDesktop).WithDetails(err.Error()).Wrap(err)
	}
	var doc struct {
		Installed *struct {
			ProjectID string `json:"project_id"`
		} `json:"installed"`
		Web *struct{} `json:"web"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", domain.Errorf(domain.KindUsage, "%s", msgNotDesktop).Wrap(err)
	}
	if doc.Installed == nil || doc.Web != nil {
		return "", domain.Errorf(domain.KindUsage, "%s", msgWebClient)
	}
	return doc.Installed.ProjectID, nil
}

// ClientSecretProjectID は、path の client_secret を検めて、installed.project_id を返す。何も保存しない。
func (a *Auth) ClientSecretProjectID(path string) (string, error) {
	_, id, err := readClientSecretFile(path)
	return id, err
}

// ImportClientSecret は、path の client_secret を検めて、暗号化して設定ディレクトリへ 0600 で置く。
// 鍵がなければここで作る（鍵を作るのはここだけ）。
func (a *Auth) ImportClientSecret(path string) error {
	data, _, err := readClientSecretFile(path)
	if err != nil {
		return err
	}
	key, created, err := a.ensureKey(context.Background())
	if err != nil {
		return err
	}
	if created {
		// 古い鍵で暗号化した token.enc は、新しい鍵では読めない。
		if err := os.Remove(a.tokenPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return domain.Errorf(domain.KindUnexpected, "古い %s を消せません", tokenName).Wrap(err)
		}
	}
	return a.writeSealedWith(key, clientSecretName, secretKindClientSecret, data)
}

func (a *Auth) loadToken() (*oauth2.Token, error) {
	data, err := a.readSealed(tokenName, secretKindToken)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

// saveToken は、トークンを暗号化して置く。鍵は作らない。
func (a *Auth) saveToken(tok *oauth2.Token) error {
	data, err := json.MarshalIndent(tok, "", "  ") //nolint:gosec // トークンの保存が目的
	if err != nil {
		return err
	}
	return a.writeSealed(tokenName, secretKindToken, data)
}

// Inspect は、保存したものが今の鍵で読めるかを調べる。KeyStore は常に埋める。
func (a *Auth) Inspect() (usecase.AuthInspection, error) {
	insp := usecase.AuthInspection{KeyStore: a.Keys.Kind()}
	hasCS, hasTok := a.HasClientSecret(), a.HasToken()
	if !hasCS && !hasTok {
		if _, _, err := a.loadKey(); err != nil { // 見つかったかは問わない。置き場が使えるかだけを見る
			return insp, err
		}
		if a.legacyExists() {
			insp.Problem = msgLegacy
		}
		return insp, nil
	}
	_, found, err := a.loadKey()
	if err != nil {
		return insp, err
	}
	if !found {
		insp.Problem = a.noKeyMessage()
		return insp, nil
	}
	if hasCS {
		data, err := a.readSealed(clientSecretName, secretKindClientSecret)
		insp.ClientSecret = err == nil
		if err != nil {
			insp.Problem = msgMismatch
		} else if pid, perr := parseClientSecret(data); perr == nil {
			insp.ProjectID = pid // 秘密ではない project_id だけを返す
		}
	}
	if hasTok {
		_, err := a.readSealed(tokenName, secretKindToken)
		insp.Token = err == nil
		if err != nil {
			insp.Problem = msgMismatch
		}
	}
	return insp, nil
}

func needLogin() *domain.Error {
	return domain.Errorf(domain.KindSetup, "ログインしていません。`youtube-cli login` を実行してください")
}

// savingSource は、更新で AccessToken が変わったら token.enc を書き直す。
type savingSource struct {
	mu    sync.Mutex
	inner oauth2.TokenSource
	last  string
	save  func(*oauth2.Token) error
}

func (s *savingSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tok, err := s.inner.Token()
	if err != nil {
		return nil, err
	}
	if tok.AccessToken != s.last {
		if err := s.save(tok); err != nil {
			if _, ok := errors.AsType[*domain.Error](err); ok {
				return nil, err
			}
			return nil, domain.Errorf(domain.KindUnexpected, "%s を保存できません", tokenName).Wrap(err)
		}
		s.last = tok.AccessToken
	}
	return tok, nil
}

// TokenSource は、token.enc から、更新したら保存し直す TokenSource を返す。
func (a *Auth) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	if !a.HasToken() {
		if fileExists(filepath.Join(a.Dir, legacyTokenName)) {
			return nil, domain.Errorf(domain.KindSetup, "%s", msgLegacy)
		}
		return nil, needLogin()
	}
	cfg, err := a.config()
	if err != nil {
		return nil, err
	}
	tok, err := a.loadToken()
	if err != nil {
		if _, ok := errors.AsType[*domain.Error](err); ok {
			return nil, err
		}
		return nil, domain.Errorf(domain.KindSetup, "%s を読めません。`youtube-cli login` を実行してください", tokenName).Wrap(err)
	}
	return &savingSource{
		inner: cfg.TokenSource(a.withHTTPClient(ctx), tok),
		last:  tok.AccessToken,
		save:  a.saveToken,
	}, nil
}

type callbackResult struct {
	code string
	err  error
}

// parseCallback は、リダイレクトのクエリから認可コードを取り出す。
func parseCallback(q url.Values, state string) (code string, ok bool, err error) {
	if q.Get("code") == "" && q.Get("error") == "" {
		return "", false, nil
	}
	if q.Get("state") != state {
		return "", true, domain.Errorf(domain.KindSetup, "state が一致しません。もう一度 `youtube-cli login` を実行してください")
	}
	if e := q.Get("error"); e != "" {
		return "", true, domain.Errorf(domain.KindSetup, "Google から認可が拒否されました: %s", e)
	}
	if c := q.Get("code"); c != "" {
		return c, true, nil
	}
	return "", false, nil
}

func newState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Login は、ブラウザ（または貼り付け）で認可コードを受け取り、token.enc に保存する。
func (a *Auth) Login(ctx context.Context, opt usecase.LoginOptions) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	timeout := a.LoginTimeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return domain.Errorf(domain.KindUnexpected, "ローカルで待ち受けられません").Wrap(err)
	}
	cfg.RedirectURL = fmt.Sprintf("http://%s/", ln.Addr().String())

	state, err := newState()
	if err != nil {
		_ = ln.Close()
		return domain.Errorf(domain.KindUnexpected, "state を作れません").Wrap(err)
	}
	verifier := oauth2.GenerateVerifier()
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"))

	results := make(chan callbackResult, 2) // 受け口ごとに高々 1 件
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code, ok, perr := parseCallback(r.URL.Query(), state)
		switch {
		case !ok:
			http.NotFound(w, r)
		case perr != nil:
			http.Error(w, "ログインできませんでした。ターミナルを確認してください。", http.StatusBadRequest)
			trySend(results, callbackResult{err: perr})
		default:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = fmt.Fprint(w, "ログインできました。このタブを閉じてください。")
			trySend(results, callbackResult{code: code})
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = srv.Serve(ln)
	}()
	defer func() {
		_ = srv.Close()
		<-served
	}()

	_, _ = fmt.Fprintf(opt.Prompt, "ブラウザで次の URL を開いてログインしてください:\n%s\n", authURL)
	if opt.NoBrowser {
		_, _ = fmt.Fprintln(opt.Prompt, "ブラウザが開けない場合は、ログイン後にアドレスバーへ表示された URL をここに貼り付けて Enter を押してください。")
		go readPasted(opt, state, results)
	} else if a.OpenBrowser != nil {
		if err := a.OpenBrowser(authURL); err != nil {
			_, _ = fmt.Fprintf(opt.Prompt, "ブラウザを開けませんでした（%v）。上の URL を手で開いてください。\n", err)
		}
	}

	var code string
	select {
	case res := <-results:
		if res.err != nil {
			return res.err
		}
		code = res.code
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return domain.Errorf(domain.KindSetup, "時間切れです。もう一度 `youtube-cli login` を実行してください")
		}
		return domain.Errorf(domain.KindUnexpected, "ログインを中断しました").Wrap(ctx.Err())
	}

	tok, err := cfg.Exchange(a.withHTTPClient(ctx), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return domain.Errorf(domain.KindSetup, "認可コードをトークンに交換できません").WithDetails(err.Error()).Wrap(err)
	}
	if err := a.saveToken(tok); err != nil {
		return err
	}
	a.removeLegacy(opt)
	return nil
}

// removeLegacy は、旧形式の平文のファイルが残っていれば消して、そのことを Prompt に書く。
func (a *Auth) removeLegacy(opt usecase.LoginOptions) {
	for _, name := range []string{legacyTokenName, legacyClientSecretName} {
		p := filepath.Join(a.Dir, name)
		if !fileExists(p) {
			continue
		}
		if err := os.Remove(p); err == nil && opt.Prompt != nil {
			_, _ = fmt.Fprintf(opt.Prompt, "古い形式のファイルを消しました: %s\n", name)
		}
	}
}

func trySend(ch chan<- callbackResult, r callbackResult) {
	select {
	case ch <- r:
	default:
	}
}

// readPasted は、Input を 1 行ずつ読み、貼り付けられた URL のクエリから認可コードを取り出す。
// 空行や、URL でない行・認可コードのない行は案内を出して読み続ける。
// 結果を送るのは、認可コードを得たときと、state の不一致や error パラメータのときだけ。
// EOF では何も送らず戻る（ループバックか時間切れの判定に任せる）。
// Input の読み取りは中断できないため、Input が閉じられるか行が届くまで残ることがある。
func readPasted(opt usecase.LoginOptions, state string, results chan<- callbackResult) {
	if opt.Input == nil {
		return
	}
	reader := bufio.NewReader(opt.Input)
	for {
		raw, readErr := reader.ReadString('\n')
		if line := strings.TrimSpace(raw); line != "" {
			u, perr := url.Parse(line)
			if perr != nil {
				_, _ = fmt.Fprintln(opt.Prompt, "URL として読めませんでした。ログイン後にアドレスバーへ表示された URL を貼り付けてください。")
			} else {
				code, ok, cerr := parseCallback(u.Query(), state)
				switch {
				case cerr != nil:
					trySend(results, callbackResult{err: cerr})
					return
				case !ok:
					_, _ = fmt.Fprintln(opt.Prompt, "その URL には認可コードがありません。ログイン後にアドレスバーへ表示された URL を貼り付けてください。")
				default:
					trySend(results, callbackResult{code: code})
					return
				}
			}
		}
		if readErr != nil {
			return
		}
	}
}

// Logout は、Google 側でトークンを取り消してから token.enc と旧 token.json を消す。鍵は消さない。
func (a *Auth) Logout(ctx context.Context) error {
	// 鍵の置き場そのものが使えないときは、何も消さずにそのエラーを返す（トークンは Google 側でまだ有効）。
	if a.HasToken() {
		if _, _, keyErr := a.loadKey(); keyErr != nil {
			return keyErr
		}
	}
	tok, err := a.loadToken()
	if errors.Is(err, os.ErrNotExist) {
		if rmErr := os.Remove(filepath.Join(a.Dir, legacyTokenName)); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return domain.Errorf(domain.KindUnexpected, "%s を消せません", legacyTokenName).Wrap(rmErr)
		}
		return nil
	}
	var revokeErr error
	if err != nil {
		revokeErr = err
	} else {
		revokeErr = a.revoke(ctx, tok)
	}
	for _, p := range []string{a.tokenPath(), filepath.Join(a.Dir, legacyTokenName)} {
		if rmErr := os.Remove(p); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return domain.Errorf(domain.KindUnexpected, "%s を消せません", filepath.Base(p)).Wrap(rmErr)
		}
	}
	if revokeErr != nil {
		return domain.Errorf(domain.KindUnexpected, "トークンは消しましたが、Google 側での取り消しに失敗しました").
			WithDetails(revokeErr.Error()).Wrap(revokeErr)
	}
	return nil
}

func (a *Auth) revoke(ctx context.Context, tok *oauth2.Token) error {
	value := tok.RefreshToken
	if value == "" {
		value = tok.AccessToken
	}
	endpoint := a.RevokeURL
	if endpoint == "" {
		endpoint = defaultRevokeURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(url.Values{"token": {value}}.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("取り消しの応答: %s", resp.Status)
	}
	return nil
}
