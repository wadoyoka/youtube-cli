package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/account"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
)

type stubAuth struct {
	token    bool
	keyStore string
	problem  string
	inspErr  error
}

func (*stubAuth) HasClientSecret() bool                             { return true }
func (*stubAuth) ImportClientSecret(string) error                   { return nil }
func (*stubAuth) ClientSecretProjectID(string) (string, error)      { return "", nil }
func (*stubAuth) Login(context.Context, usecase.LoginOptions) error { return nil }
func (s *stubAuth) Logout(context.Context) error                    { s.token = false; return nil }
func (*stubAuth) SnapshotToken() (func() error, error)              { return func() error { return nil }, nil }
func (s *stubAuth) HasToken() bool                                  { return s.token }
func (s *stubAuth) Inspect() (usecase.AuthInspection, error) {
	return usecase.AuthInspection{KeyStore: s.keyStore, ClientSecret: true, Token: s.token, Problem: s.problem}, s.inspErr
}

// testChannel は、偽の YouTube が返し、state にも保存してあるチャンネル。
var testChannel = domain.Channel{ID: "UCtest", Title: "テストチャンネル", UploadsPlaylistID: "UUtest"}

func stubRemote() usecase.Remote {
	return usecase.Remote{
		Open:   usecasetest.Opener(&usecasetest.YouTube{Channel: testChannel}),
		State:  &usecasetest.State{Saved: testChannel, Found: true},
		Locker: &usecasetest.Locker{},
	}
}

func withAuth(d Deps, loggedIn bool) Deps {
	return withStubAuth(d, &stubAuth{token: loggedIn, keyStore: "os"})
}

func withStubAuth(d Deps, a *stubAuth) Deps {
	r := stubRemote()
	d.Login = account.Login{Auth: a, Remote: r}
	d.Logout = account.Logout{Auth: a}
	d.Status = account.Status{Dir: d.Init.Dir, Auth: a, Remote: r}
	return d
}

func initAndClear(t *testing.T, d Deps, out, errb *bytes.Buffer) {
	t.Helper()
	Execute(context.Background(), d, []string{"init"})
	out.Reset()
	errb.Reset()
}

func TestStatusCommand(t *testing.T) {
	t.Run("status --json", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withAuth(d, true)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"--json", "status"})

		// Assert
		var got map[string]any
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("json: %v: %q", err, out.String())
		}
		if code != 0 || got["loggedIn"] != true || got["channelId"] != "UCtest" ||
			got["channelTitle"] != "テストチャンネル" || got["channelUrl"] != "https://www.youtube.com/channel/UCtest" || got["configDir"] == "" {
			t.Errorf("code=%d got=%v", code, got)
		}
	})

	t.Run("ログインしていない status", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withAuth(d, false)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"--json", "status"})

		// Assert
		if code != 3 || !strings.Contains(out.String(), `"loggedIn":false`) {
			t.Errorf("code=%d out=%q", code, out.String())
		}
	})

	t.Run("status の表示に鍵の置き場所と理由が出る", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withStubAuth(d, &stubAuth{keyStore: "file", problem: "鍵が合いません"})
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"status"})

		// Assert
		for _, w := range []string{"鍵の置き場所: 設定ディレクトリのファイル（key）", "理由: 鍵が合いません"} {
			if !strings.Contains(out.String(), w) {
				t.Errorf("%q が出力にない: %q", w, out.String())
			}
		}
		if code != 3 {
			t.Errorf("code=%d", code)
		}
	})

	t.Run("status の表示に OS のキーチェーンが出る", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withAuth(d, true)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"status"})

		// Assert
		if code != 0 || !strings.Contains(out.String(), "鍵の置き場所: OS のキーチェーン") || strings.Contains(out.String(), "理由:") {
			t.Errorf("code=%d out=%q", code, out.String())
		}
	})

	t.Run("status --json に鍵もトークンも含まれない", func(t *testing.T) {
		allowed := map[string]bool{
			"configDir": true, "loggedIn": true, "keyStore": true, "problem": true,
			"channelId": true, "channelTitle": true, "channelUrl": true,
		}
		const sentinel = "secret-key_SENTINEL-9f"
		cases := []struct {
			name string
			auth *stubAuth
		}{
			{"ログイン済み", &stubAuth{token: true, keyStore: "os"}},
			{"未ログインで理由あり", &stubAuth{keyStore: "file", problem: "鍵が合いません"}},
			{"鍵の置き場所のエラー", &stubAuth{keyStore: "os", inspErr: domain.Errorf(domain.KindSetup, "%s", sentinel)}},
		}
		for _, c := range cases {
			// Arrange
			d, out, errb, _ := newDeps(t)
			d = withStubAuth(d, c.auth)
			initAndClear(t, d, out, errb)

			// Act
			Execute(context.Background(), d, []string{"--json", "status"})

			// Assert
			var got map[string]any
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("%s: json: %v: %q", c.name, err, out.String())
			}
			for k := range got {
				if !allowed[k] {
					t.Errorf("%s: 想定外のキー %q", c.name, k)
				}
			}
			if _, ok := got["keyStore"]; !ok {
				t.Errorf("%s: keyStore がない", c.name)
			}
			if strings.Contains(out.String(), sentinel) {
				t.Errorf("%s: JSON にエラーの文言が含まれた: %q", c.name, out.String())
			}
		}
	})

	t.Run("拒否リストで login を拒否すると 7", func(t *testing.T) {
		// Arrange
		d, out, errb, dir := newDeps(t)
		d = withAuth(d, false)
		initAndClear(t, d, out, errb)
		if err := os.WriteFile(filepath.Join(dir, "policy.toml"), []byte(`deny = ["login"]`), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		code := Execute(context.Background(), d, []string{"login"})

		// Assert
		if code != 7 {
			t.Errorf("code=%d err=%q", code, errb.String())
		}
	})

	t.Run("login に余計な引数を渡すと 2", func(t *testing.T) {
		// Arrange
		d, _, _, _ := newDeps(t)
		d = withAuth(d, false)

		// Act
		code := Execute(context.Background(), d, []string{"login", "x"})

		// Assert
		if code != 2 {
			t.Errorf("code=%d", code)
		}
	})

	t.Run("logout は 0 を返す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withAuth(d, true)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"logout"})

		// Assert
		if code != 0 || !strings.Contains(out.String(), "ログアウト") {
			t.Errorf("code=%d out=%q", code, out.String())
		}
	})
}

func TestLoginCommand(t *testing.T) {
	other := domain.Channel{ID: "UCother", Title: "別のチャンネル", UploadsPlaylistID: "UUother"}
	withRemote := func(d Deps, a *stubAuth, yt *usecasetest.YouTube, st *usecasetest.State) Deps {
		r := usecase.Remote{Open: usecasetest.Opener(yt), State: st, Locker: &usecasetest.Locker{}}
		d.Login = account.Login{Auth: a, Remote: r}
		return d
	}

	t.Run("login はチャンネルの名前と URL を表示する", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withRemote(d, &stubAuth{keyStore: "os"}, &usecasetest.YouTube{Channel: testChannel}, &usecasetest.State{})
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"login"})

		// Assert
		want := "ログインしました: テストチャンネル\nチャンネル: https://www.youtube.com/channel/UCtest\n"
		if code != 0 || out.String() != want {
			t.Errorf("code=%d out=%q", code, out.String())
		}
	})

	t.Run("login --json はチャンネルを JSON で出す", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withRemote(d, &stubAuth{keyStore: "os"}, &usecasetest.YouTube{Channel: testChannel}, &usecasetest.State{})
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"--json", "login"})

		// Assert
		var got map[string]any
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("json: %v: %q", err, out.String())
		}
		if code != 0 || got["channelId"] != "UCtest" || got["channelTitle"] != "テストチャンネル" || got["channelUrl"] != "https://www.youtube.com/channel/UCtest" {
			t.Errorf("code=%d got=%v", code, got)
		}
	})

	t.Run("違うチャンネルでの login は 5 で止まり、--switch-channel なら 0", func(t *testing.T) {
		// Arrange
		cases := []struct {
			name string
			args []string
			want int
		}{
			{"フラグなし", []string{"login"}, 5},
			{"--switch-channel", []string{"login", "--switch-channel"}, 0},
		}
		for _, c := range cases {
			d, out, errb, _ := newDeps(t)
			d = withRemote(d, &stubAuth{keyStore: "os"}, &usecasetest.YouTube{Channel: other}, &usecasetest.State{Saved: testChannel, Found: true})
			initAndClear(t, d, out, errb)

			// Act
			code := Execute(context.Background(), d, c.args)

			// Assert
			if code != c.want {
				t.Errorf("%s: code=%d err=%q", c.name, code, errb.String())
			}
		}
	})

	t.Run("チャンネルがないアカウントの login は 3", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		noChannel := &usecasetest.YouTube{ChannelErr: domain.Errorf(domain.KindSetup, "チャンネルがありません")}
		d = withRemote(d, &stubAuth{keyStore: "os"}, noChannel, &usecasetest.State{})
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"login"})

		// Assert
		if code != 3 {
			t.Errorf("code=%d err=%q", code, errb.String())
		}
	})
}

func TestStatusCommandChannel(t *testing.T) {
	t.Run("status はチャンネルの名前・ID・URL を表示する", func(t *testing.T) {
		// Arrange
		d, out, errb, _ := newDeps(t)
		d = withAuth(d, true)
		initAndClear(t, d, out, errb)

		// Act
		code := Execute(context.Background(), d, []string{"status"})

		// Assert
		for _, w := range []string{"ログイン: 済み", "チャンネル: テストチャンネル（UCtest）", "URL: https://www.youtube.com/channel/UCtest"} {
			if !strings.Contains(out.String(), w) {
				t.Errorf("%q が出力にない: %q", w, out.String())
			}
		}
		if code != 0 {
			t.Errorf("code=%d", code)
		}
	})
}
