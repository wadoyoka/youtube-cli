package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

const testClientSecret = `{"installed":{"client_id":"test-client-id.apps.googleusercontent.com","client_secret":"test-client-secret","auth_uri":"https://accounts.example/auth","token_uri":"https://accounts.example/token","redirect_uris":["http://localhost"]}}`

var _ usecase.Authenticator = (*Auth)(nil)

type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// memKeys は、メモリ上の偽の KeyStore。本物のキーチェーンには触れない。
type memKeys struct {
	mu         sync.Mutex
	kind       string
	key        []byte
	unverified bool // LoadLocked が Unverified で返す（古い形の項目から読んだ鍵の代わり）。Load は鍵を持たない合図だけを返す
	loadErr    error
	adoptErr   error
	loads      int
	saves      int
	adopts     int
}

func (m *memKeys) Kind() string { return m.kind }

// Load は、本物と同じく、ロックの外では Unverified の鍵を渡さない（Key が nil の合図だけ）。
// Auth がロックの外で読んだ Unverified の鍵に頼らないことを、テストで確かめるため。
func (m *memKeys) Load() (LoadedKey, bool, error) {
	loaded, found, err := m.LoadLocked()
	if loaded.Unverified {
		loaded.Key = nil
	}
	return loaded, found, err
}

func (m *memKeys) LoadLocked() (LoadedKey, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loads++
	if m.loadErr != nil {
		return LoadedKey{}, false, m.loadErr
	}
	if m.key == nil {
		return LoadedKey{}, false, nil
	}
	return LoadedKey{Key: append([]byte(nil), m.key...), Unverified: m.unverified}, true, nil
}

func (m *memKeys) Save(key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saves++
	m.key = append([]byte(nil), key...)
	m.unverified = false // 置いた鍵は新しい形の項目になる
	return nil
}

func (m *memKeys) Adopt([]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.adoptErr != nil {
		return m.adoptErr
	}
	m.adopts++
	m.unverified = false
	return nil
}

func (m *memKeys) savedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saves
}

func newMemKeys() *memKeys { return &memKeys{kind: "os"} }

// fakeGoogle は、偽のトークンエンドポイントと取り消しエンドポイント。
type fakeGoogle struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	forms   []url.Values
	revoked []string
}

func newFakeGoogle(t *testing.T, tokenCheck func(url.Values) (status int, body string)) *fakeGoogle {
	t.Helper()
	g := &fakeGoogle{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		g.mu.Lock()
		g.forms = append(g.forms, r.PostForm)
		g.mu.Unlock()
		status, body := tokenCheck(r.PostForm)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
	mux.HandleFunc("/revoke", func(_ http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		g.mu.Lock()
		g.revoked = append(g.revoked, r.PostForm.Get("token"))
		g.mu.Unlock()
	})
	g.srv = httptest.NewServer(mux)
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGoogle) auth(t *testing.T) *Auth {
	t.Helper()
	dir := t.TempDir()
	return &Auth{
		Dir:          dir,
		Keys:         newMemKeys(),
		OpenBrowser:  func(string) error { return nil },
		Endpoint:     oauth2.Endpoint{AuthURL: "https://accounts.example/auth", TokenURL: g.srv.URL + "/token"},
		RevokeURL:    g.srv.URL + "/revoke",
		LoginTimeout: 5 * time.Second,
	}
}

// testKey は、a の偽の KeyStore に鍵がなければ作って返す。
func testKey(t *testing.T, a *Auth) []byte {
	t.Helper()
	m := a.Keys.(*memKeys)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.key == nil {
		k, err := newKey()
		if err != nil {
			t.Fatal(err)
		}
		m.key = k
	}
	return append([]byte(nil), m.key...)
}

func writeSealedFile(t *testing.T, a *Auth, name, kind string, plain []byte) {
	t.Helper()
	sealed, err := sealSecret(testKey(t, a), kind, plain)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Dir, name), sealed, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeClientSecret(t *testing.T, a *Auth) {
	t.Helper()
	writeSealedFile(t, a, "client_secret.enc", secretKindClientSecret, []byte(testClientSecret))
}

func writeToken(t *testing.T, a *Auth, tok *oauth2.Token) {
	t.Helper()
	data, err := json.Marshal(tok) //nolint:gosec // テストの偽トークン
	if err != nil {
		t.Fatal(err)
	}
	writeSealedFile(t, a, "token.enc", secretKindToken, data)
}

func readToken(t *testing.T, a *Auth) string {
	t.Helper()
	sealed, err := os.ReadFile(filepath.Join(a.Dir, "token.enc"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := openSecret(testKey(t, a), secretKindToken, sealed)
	if err != nil {
		t.Fatal(err)
	}
	return string(plain)
}

func writeLegacy(t *testing.T, a *Auth, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(a.Dir, name), []byte(`{"legacy":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

const okToken = `{"access_token":"a1","refresh_token":"r1","token_type":"Bearer","expires_in":3600}`

func challengeOf(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func httpGet(t *testing.T, u string) {
	t.Helper()
	resp, err := http.Get(u) //nolint:gosec,noctx // テスト内の 127.0.0.1 宛て
	if err != nil {
		t.Errorf("GET %s: %v", u, err)
		return
	}
	_ = resp.Body.Close()
}

func TestAuthLogin(t *testing.T) {
	t.Run("ループバックでログインできる", func(t *testing.T) {
		// Arrange
		var challenge string
		var cmu sync.Mutex
		g := newFakeGoogle(t, func(f url.Values) (int, string) {
			cmu.Lock()
			defer cmu.Unlock()
			if f.Get("grant_type") != "authorization_code" || f.Get("code") != "c1" ||
				challengeOf(f.Get("code_verifier")) != challenge {
				return http.StatusBadRequest, `{"error":"invalid_grant"}`
			}
			return http.StatusOK, okToken
		})
		a := g.auth(t)
		writeClientSecret(t, a)
		var browseErr []string
		a.OpenBrowser = func(raw string) error {
			u, err := url.Parse(raw)
			if err != nil {
				return err
			}
			q := u.Query()
			cmu.Lock()
			challenge = q.Get("code_challenge")
			cmu.Unlock()
			if q.Get("code_challenge_method") != "S256" {
				browseErr = append(browseErr, "code_challenge_method")
			}
			if q.Get("access_type") != "offline" {
				browseErr = append(browseErr, "access_type")
			}
			if q.Get("scope") != "https://www.googleapis.com/auth/youtube.force-ssl" {
				browseErr = append(browseErr, "scope")
			}
			if !strings.HasPrefix(q.Get("redirect_uri"), "http://127.0.0.1:") {
				browseErr = append(browseErr, "redirect_uri")
			}
			httpGet(t, q.Get("redirect_uri")+"?code=c1&state="+url.QueryEscape(q.Get("state")))
			return nil
		}
		prompt := &lockedBuffer{}

		// Act
		err := a.Login(context.Background(), usecase.LoginOptions{Prompt: prompt})
		// Assert
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if len(browseErr) > 0 {
			t.Errorf("URL の不備: %v", browseErr)
		}
		info, statErr := os.Stat(filepath.Join(a.Dir, "token.enc"))
		if statErr != nil {
			t.Fatal(statErr)
		}
		t.Run("権限は 0600", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows ではファイルの権限の数字が効かない")
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("perm = %v", info.Mode().Perm())
			}
		})
		if body := readToken(t, a); !strings.Contains(body, "r1") {
			t.Errorf("token = %s", body)
		}
		if !strings.Contains(prompt.String(), "ブラウザで次の URL を開いてログインしてください:") {
			t.Errorf("prompt = %q", prompt.String())
		}
	})

	t.Run("state が違うとログインできず、待ち受けも閉じる", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)
		writeClientSecret(t, a)
		var redirect string
		a.OpenBrowser = func(raw string) error {
			u, _ := url.Parse(raw)
			redirect = u.Query().Get("redirect_uri")
			httpGet(t, redirect+"?code=c1&state=wrong")
			return nil
		}

		// Act
		err := a.Login(context.Background(), usecase.LoginOptions{Prompt: io.Discard})

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
		if a.HasToken() {
			t.Error("token.enc ができている")
		}
		u, _ := url.Parse(redirect)
		if conn, dialErr := net.DialTimeout("tcp", u.Host, time.Second); dialErr == nil {
			_ = conn.Close()
			t.Error("待ち受けが閉じていない")
		}
	})

	t.Run("error パラメータがあると準備が必要", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)
		writeClientSecret(t, a)
		a.OpenBrowser = func(raw string) error {
			u, _ := url.Parse(raw)
			q := u.Query()
			httpGet(t, q.Get("redirect_uri")+"?error=access_denied&state="+url.QueryEscape(q.Get("state")))
			return nil
		}

		// Act
		err := a.Login(context.Background(), usecase.LoginOptions{Prompt: io.Discard})

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
	})

	t.Run("時間切れなら準備が必要", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)
		writeClientSecret(t, a)
		a.LoginTimeout = 50 * time.Millisecond

		// Act
		err := a.Login(context.Background(), usecase.LoginOptions{Prompt: io.Discard})

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
	})

	t.Run("ctx を取り消すと止まる", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)
		writeClientSecret(t, a)
		ctx, cancel := context.WithCancel(context.Background())
		a.OpenBrowser = func(string) error { cancel(); return nil }

		// Act
		err := a.Login(ctx, usecase.LoginOptions{Prompt: io.Discard})

		// Assert
		if err == nil {
			t.Fatal("エラーになるはず")
		}
		if a.HasToken() {
			t.Error("token.enc ができている")
		}
	})

	t.Run("--no-browser で貼り付けた URL からログインできる", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(f url.Values) (int, string) {
			if f.Get("code") != "c2" {
				return http.StatusBadRequest, `{"error":"invalid_grant"}`
			}
			return http.StatusOK, okToken
		})
		a := g.auth(t)
		writeClientSecret(t, a)
		a.OpenBrowser = func(string) error {
			t.Error("--no-browser なのにブラウザを開いた")
			return nil
		}
		prompt := &lockedBuffer{}
		pr, pw := io.Pipe()
		t.Cleanup(func() { _ = pw.Close() })
		done := make(chan error, 1)

		// Act
		go func() {
			done <- a.Login(context.Background(), usecase.LoginOptions{NoBrowser: true, Prompt: prompt, Input: pr})
		}()
		var state string
		deadline := time.Now().Add(5 * time.Second)
		for state == "" && time.Now().Before(deadline) {
			for f := range strings.FieldsSeq(prompt.String()) {
				if u, err := url.Parse(f); err == nil && u.Query().Get("state") != "" {
					state = u.Query().Get("state")
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		if state == "" {
			t.Fatalf("Prompt に URL がない: %q", prompt.String())
		}
		_, _ = io.WriteString(pw, "http://127.0.0.1:9/?code=c2&state="+url.QueryEscape(state)+"\n")
		err := <-done
		// Assert
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if !a.HasToken() {
			t.Error("token.enc がない")
		}
	})

	t.Run("空行のあとに正しい URL を貼ってもログインできる", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(f url.Values) (int, string) {
			if f.Get("code") != "c2" {
				return http.StatusBadRequest, `{"error":"invalid_grant"}`
			}
			return http.StatusOK, okToken
		})
		a := g.auth(t)
		writeClientSecret(t, a)
		a.OpenBrowser = func(string) error {
			t.Error("--no-browser なのにブラウザを開いた")
			return nil
		}
		prompt := &lockedBuffer{}
		pr, pw := io.Pipe()
		t.Cleanup(func() { _ = pw.Close() })
		done := make(chan error, 1)

		// Act
		go func() {
			done <- a.Login(context.Background(), usecase.LoginOptions{NoBrowser: true, Prompt: prompt, Input: pr})
		}()
		var state string
		deadline := time.Now().Add(5 * time.Second)
		for state == "" && time.Now().Before(deadline) {
			for f := range strings.FieldsSeq(prompt.String()) {
				if u, err := url.Parse(f); err == nil && u.Query().Get("state") != "" {
					state = u.Query().Get("state")
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		if state == "" {
			t.Fatalf("Prompt に URL がない: %q", prompt.String())
		}
		_, _ = io.WriteString(pw, "\n  \n")
		_, _ = io.WriteString(pw, "http://127.0.0.1:9/?code=c2&state="+url.QueryEscape(state)+"\n")
		err := <-done
		// Assert
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if !a.HasToken() {
			t.Error("token.enc がない")
		}
	})

	t.Run("URL でない行のあとに正しい URL を貼ってもログインできる", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(f url.Values) (int, string) {
			if f.Get("code") != "c2" {
				return http.StatusBadRequest, `{"error":"invalid_grant"}`
			}
			return http.StatusOK, okToken
		})
		a := g.auth(t)
		writeClientSecret(t, a)
		a.OpenBrowser = func(string) error {
			t.Error("--no-browser なのにブラウザを開いた")
			return nil
		}
		prompt := &lockedBuffer{}
		pr, pw := io.Pipe()
		t.Cleanup(func() { _ = pw.Close() })
		done := make(chan error, 1)

		// Act
		go func() {
			done <- a.Login(context.Background(), usecase.LoginOptions{NoBrowser: true, Prompt: prompt, Input: pr})
		}()
		var state string
		deadline := time.Now().Add(5 * time.Second)
		for state == "" && time.Now().Before(deadline) {
			for f := range strings.FieldsSeq(prompt.String()) {
				if u, err := url.Parse(f); err == nil && u.Query().Get("state") != "" {
					state = u.Query().Get("state")
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		if state == "" {
			t.Fatalf("Prompt に URL がない: %q", prompt.String())
		}
		_, _ = io.WriteString(pw, "garbage\nhttp://127.0.0.1:9/?nothing=1\n")
		_, _ = io.WriteString(pw, "http://127.0.0.1:9/?code=c2&state="+url.QueryEscape(state)+"\n")
		err := <-done
		// Assert
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if !a.HasToken() {
			t.Error("token.enc がない")
		}
	})

	t.Run("client_secret がなければ準備が必要", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)

		// Act
		err := a.Login(context.Background(), usecase.LoginOptions{Prompt: io.Discard})

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
	})
}

func TestAuthImportClientSecret(t *testing.T) {
	t.Run("壊れた JSON は引数の誤り", func(t *testing.T) {
		// Arrange
		a := &Auth{Dir: t.TempDir(), Keys: newMemKeys()}
		src := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(src, []byte(`{not json`), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		err := a.ImportClientSecret(src)

		// Assert
		if domain.KindOf(err) != domain.KindUsage {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
		if a.HasClientSecret() {
			t.Error("コピーされている")
		}
	})

	t.Run("読めないパスは引数の誤り", func(t *testing.T) {
		// Arrange
		a := &Auth{Dir: t.TempDir(), Keys: newMemKeys()}

		// Act
		err := a.ImportClientSecret(filepath.Join(t.TempDir(), "none.json"))

		// Assert
		if domain.KindOf(err) != domain.KindUsage {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
	})

	t.Run("正しい installed 形式なら暗号化して 0600 で置く", func(t *testing.T) {
		// Arrange
		a := &Auth{Dir: t.TempDir(), Keys: newMemKeys()}
		src := filepath.Join(t.TempDir(), "cs.json")
		if err := os.WriteFile(src, []byte(testClientSecret), 0o644); err != nil { //nolint:gosec // 元ファイルは緩い権限でよい
			t.Fatal(err)
		}

		// Act
		err := a.ImportClientSecret(src)
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(a.Dir, "client_secret.enc")
		info, statErr := os.Stat(dst)
		if statErr != nil {
			t.Fatal(statErr)
		}
		t.Run("権限は 0600", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows ではファイルの権限の数字が効かない")
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("perm = %v", info.Mode().Perm())
			}
		})
		body, _ := os.ReadFile(dst) //nolint:gosec // テストの一時ファイル
		plain, openErr := openSecret(testKey(t, a), secretKindClientSecret, body)
		if openErr != nil || string(plain) != testClientSecret {
			t.Errorf("復号 = %s, err = %v", plain, openErr)
		}
		if !a.HasClientSecret() {
			t.Error("HasClientSecret = false")
		}
	})
}

func TestAuthTokenSource(t *testing.T) {
	t.Run("トークンも client_secret もなければ準備が必要", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)

		// Act
		_, err := a.TokenSource(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup || !strings.Contains(err.Error(), "youtube-cli login") {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
	})

	t.Run("更新したトークンを保存し直す", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(f url.Values) (int, string) {
			if f.Get("grant_type") != "refresh_token" || f.Get("refresh_token") != "r1" {
				return http.StatusBadRequest, `{"error":"invalid_grant"}`
			}
			return http.StatusOK, `{"access_token":"a2","token_type":"Bearer","expires_in":3600}`
		})
		a := g.auth(t)
		writeClientSecret(t, a)
		writeToken(t, a, &oauth2.Token{
			AccessToken: "a0", RefreshToken: "r1", TokenType: "Bearer",
			Expiry: time.Now().Add(-time.Hour),
		})

		// Act
		ts, err := a.TokenSource(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		tok, tokErr := ts.Token()

		// Assert
		if tokErr != nil {
			t.Fatal(tokErr)
		}
		if tok.AccessToken != "a2" {
			t.Errorf("AccessToken = %q", tok.AccessToken)
		}
		if body := readToken(t, a); !strings.Contains(body, "a2") || !strings.Contains(body, "r1") {
			t.Errorf("token = %s", body)
		}
	})
}

func TestAuthLogout(t *testing.T) {
	t.Run("取り消してからトークンを消す", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)
		writeToken(t, a, &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"})

		// Act
		err := a.Logout(context.Background())
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		g.mu.Lock()
		revoked := append([]string(nil), g.revoked...)
		g.mu.Unlock()
		if len(revoked) != 1 || revoked[0] != "r1" {
			t.Errorf("revoked = %v", revoked)
		}
		if a.HasToken() {
			t.Error("token.enc が残っている")
		}
	})

	t.Run("取り消しに失敗してもファイルは消し、エラーを返す", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)
		a.RevokeURL = g.srv.URL + "/missing"
		writeToken(t, a, &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"})

		// Act
		err := a.Logout(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindUnexpected {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
		if a.HasToken() {
			t.Error("token.enc が残っている")
		}
	})

	t.Run("トークンがなければ何もしない", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)

		// Act
		err := a.Logout(context.Background())

		// Assert
		g.mu.Lock()
		n := len(g.revoked)
		g.mu.Unlock()
		if err != nil || n != 0 {
			t.Errorf("err = %v, revoked = %d", err, n)
		}
	})
}

func TestAuthClientSecretProjectID(t *testing.T) {
	t.Run("ClientSecretProjectID は installed.project_id を返し、ファイルを保存しない", func(t *testing.T) {
		// Arrange
		keys := newMemKeys()
		a := &Auth{Dir: t.TempDir(), Keys: keys}
		src := filepath.Join(t.TempDir(), "cs.json")
		body := strings.Replace(testClientSecret, `"client_id"`, `"project_id":"my-proj-123","client_id"`, 1)
		if err := os.WriteFile(src, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		got, err := a.ClientSecretProjectID(src)

		// Assert
		if err != nil || got != "my-proj-123" {
			t.Fatalf("got %q, %v", got, err)
		}
		if a.HasClientSecret() || keys.savedCount() != 0 {
			t.Error("保存している")
		}
	})

	t.Run("project_id がなければ空文字", func(t *testing.T) {
		// Arrange
		a := &Auth{Dir: t.TempDir(), Keys: newMemKeys()}

		// Act
		got, err := a.ClientSecretProjectID(writeSrcSecret(t))

		// Assert
		if err != nil || got != "" {
			t.Fatalf("got %q, %v", got, err)
		}
	})

	t.Run("壊れた JSON と読めないパスは KindUsage", func(t *testing.T) {
		// Arrange
		a := &Auth{Dir: t.TempDir(), Keys: newMemKeys()}
		bad := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(bad, []byte(`{not json`), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		_, err1 := a.ClientSecretProjectID(bad)
		_, err2 := a.ClientSecretProjectID(filepath.Join(t.TempDir(), "none.json"))

		// Assert
		if domain.KindOf(err1) != domain.KindUsage || domain.KindOf(err2) != domain.KindUsage {
			t.Errorf("err1=%v err2=%v", err1, err2)
		}
	})
}

// webMarker は、web のクライアントの JSON の client_secret に入れる目印。エラーに出ていないことを確かめる。
const webMarker = "zq-web-marker-7f3k"

const webClientSecret = `{"web":{"client_id":"test-web-id.apps.googleusercontent.com","project_id":"web-proj-1","client_secret":"` + webMarker + `","auth_uri":"https://accounts.example/auth","token_uri":"https://accounts.example/token","redirect_uris":["https://example.com/cb"]}}`

func TestAuthRejectsNonDesktopClient(t *testing.T) {
	const guide = "デスクトップアプリの OAuth クライアントの JSON を指定してください"
	installedPart := strings.TrimSuffix(strings.TrimPrefix(testClientSecret, "{"), "}")
	webPart := strings.TrimSuffix(strings.TrimPrefix(webClientSecret, "{"), "}")
	bodies := map[string]string{
		"web だけ":              webClientSecret,
		"installed と web の両方": "{" + installedPart + "," + webPart + "}",
	}
	for name, body := range bodies {
		t.Run(name+"の JSON は、取り込みも project_id の検めも KindUsage で断り、何も保存しない", func(t *testing.T) {
			// Arrange
			keys := newMemKeys()
			a := &Auth{Dir: t.TempDir(), Keys: keys}
			src := filepath.Join(t.TempDir(), "web.json")
			if err := os.WriteFile(src, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}

			// Act
			importErr := a.ImportClientSecret(src)
			pid, checkErr := a.ClientSecretProjectID(src)

			// Assert
			for _, err := range []error{importErr, checkErr} {
				if domain.KindOf(err) != domain.KindUsage || !strings.Contains(err.Error(), guide) {
					t.Errorf("kind = %v, err = %v", domain.KindOf(err), err)
				}
				var de *domain.Error
				if errors.As(err, &de) && (strings.Contains(err.Error(), webMarker) || strings.Contains(strings.Join(de.Details, " "), webMarker)) {
					t.Errorf("エラーにシークレットが出ている: %v", err)
				}
			}
			if pid != "" {
				t.Errorf("pid = %q", pid)
			}
			if a.HasClientSecret() || keys.savedCount() != 0 {
				t.Error("保存している")
			}
		})
	}

	t.Run("installed.project_id が文字列でない JSON は、デスクトップアプリの案内つきの KindUsage", func(t *testing.T) {
		// Arrange
		a := &Auth{Dir: t.TempDir(), Keys: newMemKeys()}
		src := filepath.Join(t.TempDir(), "cs.json")
		body := strings.Replace(testClientSecret, `"client_id"`, `"project_id":123,"client_id"`, 1)
		if err := os.WriteFile(src, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		_, err := a.ClientSecretProjectID(src)

		// Assert
		if domain.KindOf(err) != domain.KindUsage || !strings.Contains(err.Error(), guide) {
			t.Errorf("kind = %v, err = %v", domain.KindOf(err), err)
		}
	})
}

func TestAuthInspectProjectID(t *testing.T) {
	withProject := strings.Replace(testClientSecret, `"client_id"`, `"project_id":"my-proj-123","client_id"`, 1)

	t.Run("client_secret.enc を今の鍵で読めたら、その installed.project_id を返す", func(t *testing.T) {
		// Arrange
		a := &Auth{Dir: t.TempDir(), Keys: newMemKeys()}
		writeSealedFile(t, a, "client_secret.enc", secretKindClientSecret, []byte(withProject))

		// Act
		got, err := a.Inspect()
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if !got.ClientSecret || got.ProjectID != "my-proj-123" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("鍵が合わなければ project_id は空", func(t *testing.T) {
		// Arrange
		a := &Auth{Dir: t.TempDir(), Keys: newMemKeys()}
		writeSealedFile(t, a, "client_secret.enc", secretKindClientSecret, []byte(withProject))
		other, _ := newKey()
		a.Keys.(*memKeys).key = other

		// Act
		got, err := a.Inspect()
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if got.ClientSecret || got.ProjectID != "" {
			t.Errorf("got %+v", got)
		}
	})
}

func TestAuthNoKeyHint(t *testing.T) {
	const hint = "`YOUTUBE_CLI_KEYRING=file` を付け忘れていませんか"

	// noKeyAuth は、client_secret.enc と token.enc があり、鍵だけがない Auth を返す。
	noKeyAuth := func(t *testing.T, kind string, keyFile bool) *Auth {
		t.Helper()
		a := &Auth{Dir: t.TempDir(), Keys: &memKeys{kind: kind}}
		writeClientSecret(t, a)
		writeToken(t, a, &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"})
		a.Keys.(*memKeys).key = nil
		if keyFile {
			if err := os.WriteFile(filepath.Join(a.Dir, "key"), []byte("not-a-real-key"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return a
	}

	t.Run("OS のキーチェーンに鍵がなく、設定ディレクトリに key があれば、Inspect の理由に付け忘れの心当たりを足す", func(t *testing.T) {
		// Arrange
		a := noKeyAuth(t, "os", true)

		// Act
		got, err := a.Inspect()
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if got.Token || !strings.Contains(got.Problem, "鍵が見つかりません") || !strings.Contains(got.Problem, hint) {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("同じとき、TokenSource のエラーにも付け忘れの心当たりを足す", func(t *testing.T) {
		// Arrange
		a := noKeyAuth(t, "os", true)

		// Act
		_, err := a.TokenSource(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup || !strings.Contains(err.Error(), hint) {
			t.Errorf("kind = %v, err = %v", domain.KindOf(err), err)
		}
	})

	for _, tc := range []struct {
		name    string
		kind    string
		keyFile bool
	}{
		{"設定ディレクトリに key がなければ", "os", false},
		{"鍵の置き場所が file なら", "file", true},
	} {
		t.Run(tc.name+"、付け忘れの心当たりは足さない", func(t *testing.T) {
			// Arrange
			a := noKeyAuth(t, tc.kind, tc.keyFile)

			// Act
			got, err := a.Inspect()
			// Assert
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got.Problem, "鍵が見つかりません") || strings.Contains(got.Problem, "YOUTUBE_CLI_KEYRING") {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func writeSrcSecret(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "cs.json")
	if err := os.WriteFile(src, []byte(testClientSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestAuthKeyCreation(t *testing.T) {
	t.Run("取り込むと client_secret.enc が暗号化されて置かれ、平文が含まれない", func(t *testing.T) {
		// Arrange
		a := &Auth{Dir: t.TempDir(), Keys: newMemKeys()}

		// Act
		err := a.ImportClientSecret(writeSrcSecret(t))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		body, _ := os.ReadFile(filepath.Join(a.Dir, "client_secret.enc"))
		if bytes.Contains(body, []byte("test-client-id")) || bytes.Contains(body, []byte("test-client-secret")) {
			t.Errorf("平文が含まれる: %s", body)
		}
	})

	t.Run("鍵がなければ取り込みで鍵を作る", func(t *testing.T) {
		// Arrange
		keys := newMemKeys()
		a := &Auth{Dir: t.TempDir(), Keys: keys}

		// Act
		err := a.ImportClientSecret(writeSrcSecret(t))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if keys.savedCount() != 1 || len(keys.key) != keyLength {
			t.Errorf("saves = %d, key = %d バイト", keys.savedCount(), len(keys.key))
		}
	})

	t.Run("鍵があり、ファイルを復号できれば作り直さない", func(t *testing.T) {
		// Arrange
		keys := newMemKeys()
		a := &Auth{Dir: t.TempDir(), Keys: keys}
		writeClientSecret(t, a)

		// Act
		err := a.ImportClientSecret(writeSrcSecret(t))

		// Assert
		if err != nil || keys.savedCount() != 0 {
			t.Errorf("err = %v, saves = %d", err, keys.savedCount())
		}
	})

	t.Run("鍵があってもファイルがなければ作り直す", func(t *testing.T) {
		// Arrange
		keys := newMemKeys()
		a := &Auth{Dir: t.TempDir(), Keys: keys}
		old := testKey(t, a)

		// Act
		err := a.ImportClientSecret(writeSrcSecret(t))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if keys.savedCount() != 1 {
			t.Errorf("saves = %d", keys.savedCount())
		}
		if bytes.Equal(keys.key, old) {
			t.Error("元の鍵のまま")
		}
	})

	t.Run("鍵があってもファイルを復号できなければ作り直し、token.enc を消す", func(t *testing.T) {
		// Arrange
		keys := newMemKeys()
		a := &Auth{Dir: t.TempDir(), Keys: keys}
		writeClientSecret(t, a)
		writeToken(t, a, &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"})
		other, err := newKey()
		if err != nil {
			t.Fatal(err)
		}
		keys.key = other // ファイルは別の鍵で封をしてある

		// Act
		err = a.ImportClientSecret(writeSrcSecret(t))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if keys.savedCount() != 1 || bytes.Equal(keys.key, other) {
			t.Errorf("saves = %d, 鍵を置き換えたか = %v", keys.savedCount(), !bytes.Equal(keys.key, other))
		}
		if a.HasToken() {
			t.Error("token.enc が残っている")
		}
		if _, cfgErr := a.config(); cfgErr != nil {
			t.Errorf("新しい client_secret.enc を新しい鍵で読めない: %v", cfgErr)
		}
	})

	t.Run("鍵を新しく作ったときは古い token.enc を消す", func(t *testing.T) {
		// Arrange
		a := &Auth{Dir: t.TempDir(), Keys: newMemKeys()}
		if err := os.WriteFile(filepath.Join(a.Dir, "token.enc"), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		err := a.ImportClientSecret(writeSrcSecret(t))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if a.HasToken() {
			t.Error("token.enc が残っている")
		}
	})

	t.Run("キーチェーンが使えなければ取り込みは KindSetup で、鍵を作らない", func(t *testing.T) {
		// Arrange
		keys := newMemKeys()
		keys.loadErr = domain.Errorf(domain.KindSetup, "キーチェーンが使えません")
		a := &Auth{Dir: t.TempDir(), Keys: keys}

		// Act
		err := a.ImportClientSecret(writeSrcSecret(t))

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
		if keys.savedCount() != 0 {
			t.Error("鍵を作ってしまった")
		}
		if a.HasClientSecret() {
			t.Error("client_secret.enc ができている")
		}
	})

	t.Run("鍵を作るときはロックを取る", func(t *testing.T) {
		// Arrange
		var locked, unlocked int
		a := &Auth{Dir: t.TempDir(), Keys: newMemKeys(), Lock: func(context.Context) (func(), error) {
			locked++
			return func() { unlocked++ }, nil
		}}

		// Act
		err := a.ImportClientSecret(writeSrcSecret(t))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if locked != 1 || unlocked != 1 {
			t.Errorf("locked = %d, unlocked = %d", locked, unlocked)
		}
	})

	t.Run("ロックを取れなければ鍵を作らない", func(t *testing.T) {
		// Arrange
		keys := newMemKeys()
		a := &Auth{Dir: t.TempDir(), Keys: keys, Lock: func(context.Context) (func(), error) {
			return nil, errors.New("取れない")
		}}

		// Act
		err := a.ImportClientSecret(writeSrcSecret(t))

		// Assert
		if err == nil || keys.savedCount() != 0 {
			t.Errorf("err = %v, saves = %d", err, keys.savedCount())
		}
	})

	t.Run("古い形の鍵でファイルを復号できれば、取り込みで移し、作り直さない。ロックは1回", func(t *testing.T) {
		// Arrange
		keys := newMemKeys()
		var locked, unlocked int
		a := &Auth{Dir: t.TempDir(), Keys: keys, Lock: func(context.Context) (func(), error) {
			locked++
			return func() { unlocked++ }, nil
		}}
		writeClientSecret(t, a)
		keys.unverified = true

		// Act
		err := a.ImportClientSecret(writeSrcSecret(t))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if keys.adopts != 1 || keys.savedCount() != 0 {
			t.Errorf("adopts = %d, saves = %d", keys.adopts, keys.savedCount())
		}
		if locked != 1 || unlocked != 1 {
			t.Errorf("locked = %d, unlocked = %d", locked, unlocked)
		}
	})

	t.Run("YOUTUBE_CLI_KEYRING=file で key が合わなければ、取り込みで key を置き換える", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		a := &Auth{Dir: dir, Keys: newFileKeyStore(dir)}
		stale, err := newKey()
		if err != nil {
			t.Fatal(err)
		}
		if err := a.Keys.Save(stale); err != nil {
			t.Fatal(err)
		}
		sealer, err := newKey()
		if err != nil {
			t.Fatal(err)
		}
		sealed, err := sealSecret(sealer, secretKindClientSecret, []byte(testClientSecret))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "client_secret.enc"), sealed, 0o600); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(filepath.Join(dir, "key")) //nolint:gosec // テストの一時ファイル
		if err != nil {
			t.Fatal(err)
		}

		// Act
		err = a.ImportClientSecret(writeSrcSecret(t))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(filepath.Join(dir, "key")) //nolint:gosec // テストの一時ファイル
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(before, after) {
			t.Error("key が置き換わっていない")
		}
		if _, cfgErr := a.config(); cfgErr != nil {
			t.Errorf("新しい client_secret.enc を新しい key で読めない: %v", cfgErr)
		}
	})
}

func TestAuthKeyAdoption(t *testing.T) {
	// oldKeyAuth は、古い形の項目から読んだ鍵（Unverified）で、client_secret.enc と token.enc を復号できる Auth を返す。
	oldKeyAuth := func(t *testing.T, lock func(context.Context) (func(), error)) (*Auth, *memKeys) {
		t.Helper()
		keys := newMemKeys()
		a := &Auth{Dir: t.TempDir(), Keys: keys, Lock: lock}
		writeClientSecret(t, a)
		writeToken(t, a, &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"})
		keys.unverified = true
		return a, keys
	}

	t.Run("古い形の鍵でファイルを復号できれば、読むときに移し替える", func(t *testing.T) {
		// Arrange
		var locked, unlocked int
		a, keys := oldKeyAuth(t, func(context.Context) (func(), error) {
			locked++
			return func() { unlocked++ }, nil
		})

		// Act
		_, err := a.TokenSource(context.Background())
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if keys.adopts != 1 {
			t.Errorf("adopts = %d", keys.adopts)
		}
		if locked != 1 || unlocked != 1 {
			t.Errorf("locked = %d, unlocked = %d", locked, unlocked)
		}
	})

	t.Run("token.enc だけでも、復号できれば移し替える", func(t *testing.T) {
		// Arrange
		keys := newMemKeys()
		a := &Auth{Dir: t.TempDir(), Keys: keys}
		writeToken(t, a, &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"})
		keys.unverified = true

		// Act
		key, found, err := a.loadKey()
		// Assert
		if err != nil || !found || key == nil {
			t.Fatalf("found = %v, err = %v", found, err)
		}
		if keys.adopts != 1 {
			t.Errorf("adopts = %d", keys.adopts)
		}
	})

	t.Run("古い形の鍵でファイルを復号できなければ移さず、合わない文言", func(t *testing.T) {
		// Arrange
		a, keys := oldKeyAuth(t, nil)
		other, err := newKey()
		if err != nil {
			t.Fatal(err)
		}
		keys.key = other // ファイルは別の鍵で封をしてある

		// Act
		_, tsErr := a.TokenSource(context.Background())
		insp, inspErr := a.Inspect()

		// Assert
		if domain.KindOf(tsErr) != domain.KindSetup || !strings.Contains(tsErr.Error(), "今の鍵で読めません") {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(tsErr), tsErr)
		}
		if keys.adopts != 0 || a.key != nil {
			t.Errorf("adopts = %d, メモリに持ったか = %v", keys.adopts, a.key != nil)
		}
		if inspErr != nil || insp.Problem != msgMismatch {
			t.Errorf("Inspect = %+v, err = %v", insp, inspErr)
		}
	})

	t.Run("移し替えに失敗したら KindSetup で止まり、その鍵を使わない", func(t *testing.T) {
		// Arrange
		a, keys := oldKeyAuth(t, nil)
		keys.adoptErr = domain.Errorf(domain.KindSetup, "キーチェーンが使えません")

		// Act
		_, err := a.TokenSource(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
		if a.key != nil {
			t.Error("移せなかった鍵をメモリに持っている")
		}
	})

	t.Run("移し替えのロックを取れなければ止まり、移さない", func(t *testing.T) {
		// Arrange
		a, keys := oldKeyAuth(t, func(context.Context) (func(), error) {
			return nil, errors.New("取れない")
		})

		// Act
		_, err := a.TokenSource(context.Background())

		// Assert
		if err == nil {
			t.Fatal("エラーにならない")
		}
		if keys.adopts != 0 {
			t.Errorf("adopts = %d", keys.adopts)
		}
	})

	t.Run("ロックを待つ間にほかの youtube-cli が移し終えていれば、Adopt を呼ばずにその鍵を使う", func(t *testing.T) {
		// Arrange
		var keys *memKeys
		a, keys := oldKeyAuth(t, func(context.Context) (func(), error) {
			keys.mu.Lock()
			keys.unverified = false // ほかの youtube-cli が移し終えた
			keys.mu.Unlock()
			return func() {}, nil
		})

		// Act
		_, err := a.TokenSource(context.Background())
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if keys.adopts != 0 {
			t.Errorf("adopts = %d", keys.adopts)
		}
	})
}

func TestAuthEncryptedToken(t *testing.T) {
	t.Run("ログインのあと token.enc は暗号化されていて、平文のアクセストークンを含まない", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) {
			return http.StatusOK, `{"access_token":"access-tok-sentinel","refresh_token":"refresh-tok-sentinel","token_type":"Bearer","expires_in":3600}`
		})
		a := g.auth(t)
		writeClientSecret(t, a)
		a.OpenBrowser = func(raw string) error {
			u, _ := url.Parse(raw)
			q := u.Query()
			httpGet(t, q.Get("redirect_uri")+"?code=c1&state="+url.QueryEscape(q.Get("state")))
			return nil
		}

		// Act
		err := a.Login(context.Background(), usecase.LoginOptions{Prompt: io.Discard})
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		body, _ := os.ReadFile(filepath.Join(a.Dir, "token.enc"))
		if bytes.Contains(body, []byte("access-tok")) || bytes.Contains(body, []byte("refresh-tok")) {
			t.Errorf("平文が含まれる: %s", body)
		}
	})

	t.Run("ログインに成功したら古い平文のファイルを消して、そのことを表示する", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)
		writeClientSecret(t, a)
		writeLegacy(t, a, "token.json")
		writeLegacy(t, a, "client_secret.json")
		a.OpenBrowser = func(raw string) error {
			u, _ := url.Parse(raw)
			q := u.Query()
			httpGet(t, q.Get("redirect_uri")+"?code=c1&state="+url.QueryEscape(q.Get("state")))
			return nil
		}
		prompt := &lockedBuffer{}

		// Act
		err := a.Login(context.Background(), usecase.LoginOptions{Prompt: prompt})
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"token.json", "client_secret.json"} {
			if fileExists(filepath.Join(a.Dir, name)) {
				t.Errorf("%s が残っている", name)
			}
			if !strings.Contains(prompt.String(), "古い形式のファイルを消しました: "+name) {
				t.Errorf("%s の表示がない: %q", name, prompt.String())
			}
		}
	})

	t.Run("トークンの更新の保存では鍵を作らない", func(t *testing.T) {
		// Arrange
		keys := newMemKeys()
		a := &Auth{Dir: t.TempDir(), Keys: keys}

		// Act
		err := a.saveToken(&oauth2.Token{AccessToken: "a2", RefreshToken: "r1"})

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
		if keys.savedCount() != 0 || a.HasToken() {
			t.Errorf("saves = %d, HasToken = %v", keys.savedCount(), a.HasToken())
		}
	})

	t.Run("更新のあとの保存で鍵が消えていたら KindSetup", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) {
			return http.StatusOK, `{"access_token":"a2","token_type":"Bearer","expires_in":3600}`
		})
		a := g.auth(t)
		writeClientSecret(t, a)
		writeToken(t, a, &oauth2.Token{AccessToken: "a0", RefreshToken: "r1", Expiry: time.Now().Add(-time.Hour)})
		cfg, err := a.config()
		if err != nil {
			t.Fatal(err)
		}
		tok, err := a.loadToken()
		if err != nil {
			t.Fatal(err)
		}
		other := &Auth{Dir: a.Dir, Keys: newMemKeys()} // 鍵のない別の Auth
		src := &savingSource{inner: cfg.TokenSource(context.Background(), tok), last: tok.AccessToken, save: other.saveToken}

		// Act
		_, tokErr := src.Token()

		// Assert
		if domain.KindOf(tokErr) != domain.KindSetup {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(tokErr), tokErr)
		}
	})

	t.Run("古い平文のファイルだけなら TokenSource は KindSetup で古い形式の文言", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)
		writeLegacy(t, a, "token.json")
		writeLegacy(t, a, "client_secret.json")

		// Act
		_, err := a.TokenSource(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup || !strings.Contains(err.Error(), "古い形式のファイルがあります") {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
	})

	t.Run("鍵が合わなければ TokenSource は KindSetup で、合わない文言", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)
		writeClientSecret(t, a)
		writeToken(t, a, &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"})
		other, _ := newKey()
		a.Keys.(*memKeys).key = other

		// Act
		_, err := a.TokenSource(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup || !strings.Contains(err.Error(), "今の鍵で読めません") {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
	})

	t.Run("鍵がなければ TokenSource は KindSetup で、鍵がない文言", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)
		writeClientSecret(t, a)
		writeToken(t, a, &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"})
		a.Keys.(*memKeys).key = nil

		// Act
		_, err := a.TokenSource(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup || !strings.Contains(err.Error(), "鍵が見つかりません") {
			t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
		}
	})
}

func TestAuthInspect(t *testing.T) {
	const (
		noKey    = "鍵が見つかりません。`youtube-cli login --client-secret <JSON>` をやり直してください"
		mismatch = "今の鍵で読めません。`youtube-cli login --client-secret <JSON>` をやり直してください"
		legacy   = "古い形式のファイルがあります。`youtube-cli login --client-secret <JSON>` をやり直してください"
	)
	tests := []struct {
		name    string
		setup   func(t *testing.T, a *Auth)
		want    usecase.AuthInspection
		wantErr domain.ErrorKind
		hasErr  bool
	}{
		{"何もない", func(*testing.T, *Auth) {}, usecase.AuthInspection{KeyStore: "os"}, 0, false},
		{"両方読める", func(t *testing.T, a *Auth) {
			writeClientSecret(t, a)
			writeToken(t, a, &oauth2.Token{AccessToken: "a1"})
		}, usecase.AuthInspection{KeyStore: "os", ClientSecret: true, Token: true}, 0, false},
		{"鍵がない", func(t *testing.T, a *Auth) {
			writeClientSecret(t, a)
			writeToken(t, a, &oauth2.Token{AccessToken: "a1"})
			a.Keys.(*memKeys).key = nil
		}, usecase.AuthInspection{KeyStore: "os", Problem: noKey}, 0, false},
		{"鍵が合わない", func(t *testing.T, a *Auth) {
			writeClientSecret(t, a)
			writeToken(t, a, &oauth2.Token{AccessToken: "a1"})
			other, _ := newKey()
			a.Keys.(*memKeys).key = other
		}, usecase.AuthInspection{KeyStore: "os", Problem: mismatch}, 0, false},
		{"古い形式だけ", func(t *testing.T, a *Auth) {
			writeLegacy(t, a, "token.json")
			writeLegacy(t, a, "client_secret.json")
		}, usecase.AuthInspection{KeyStore: "os", Problem: legacy}, 0, false},
		{"何もなくて不正な YOUTUBE_CLI_KEYRING", func(_ *testing.T, a *Auth) {
			a.Keys = invalidKeyStore{mode: "fiel"}
		}, usecase.AuthInspection{KeyStore: "fiel"}, domain.KindUsage, true},
		{"何もなくて KeyStore が使えない", func(_ *testing.T, a *Auth) {
			a.Keys.(*memKeys).loadErr = domain.Errorf(domain.KindSetup, "キーチェーンが使えません")
		}, usecase.AuthInspection{KeyStore: "os"}, domain.KindSetup, true},
		{"KeyStore が使えない", func(t *testing.T, a *Auth) {
			writeClientSecret(t, a)
			a.Keys.(*memKeys).loadErr = domain.Errorf(domain.KindSetup, "キーチェーンが使えません")
		}, usecase.AuthInspection{KeyStore: "os"}, domain.KindSetup, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			a := &Auth{Dir: t.TempDir(), Keys: newMemKeys()}
			tt.setup(t, a)

			// Act
			got, err := a.Inspect()

			// Assert
			if tt.hasErr {
				if domain.KindOf(err) != tt.wantErr {
					t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAuthLogoutEncrypted(t *testing.T) {
	t.Run("ログアウトは token.enc と古い token.json を消し、鍵は消さない", func(t *testing.T) {
		// Arrange
		g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
		a := g.auth(t)
		writeToken(t, a, &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"})
		writeLegacy(t, a, "token.json")

		// Act
		err := a.Logout(context.Background())
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if a.HasToken() || fileExists(filepath.Join(a.Dir, "token.json")) {
			t.Error("トークンのファイルが残っている")
		}
		if a.Keys.(*memKeys).key == nil {
			t.Error("鍵が消えた")
		}
	})
}

func TestAuthLogoutKeyStoreFailure(t *testing.T) {
	tests := []struct {
		name string
		keys func() KeyStore
		kind domain.ErrorKind
	}{
		{"キーチェーンが使えなければ KindSetup", func() KeyStore {
			m := newMemKeys()
			m.loadErr = domain.Errorf(domain.KindSetup, "キーチェーンが使えません")
			return m
		}, domain.KindSetup},
		{"不正な YOUTUBE_CLI_KEYRING なら KindUsage", func() KeyStore { return invalidKeyStore{mode: "fiel"} }, domain.KindUsage},
	}
	for _, tt := range tests {
		t.Run("ログアウト: "+tt.name+"で、token.enc を消さず取り消しもしない", func(t *testing.T) {
			// Arrange
			g := newFakeGoogle(t, func(url.Values) (int, string) { return http.StatusOK, okToken })
			a := g.auth(t)
			writeToken(t, a, &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"})
			a.Keys = tt.keys()

			// Act
			err := a.Logout(context.Background())

			// Assert
			if domain.KindOf(err) != tt.kind {
				t.Fatalf("kind = %v, err = %v", domain.KindOf(err), err)
			}
			if !a.HasToken() {
				t.Error("token.enc を消してしまった")
			}
			g.mu.Lock()
			n := len(g.revoked)
			g.mu.Unlock()
			if n != 0 {
				t.Errorf("revoked = %d", n)
			}
		})
	}
}
