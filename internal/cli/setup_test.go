package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/account"
	"github.com/wadoyoka/youtube-cli/internal/usecase/setup"
)

// recCloud は、呼ばれたことを記録する偽の gcloud。
type recCloud struct {
	calls []string
}

func (c *recCloud) Installed() bool { c.calls = append(c.calls, "Installed"); return true }
func (c *recCloud) ActiveAccount(context.Context) (string, error) {
	c.calls = append(c.calls, "ActiveAccount")
	return "me@example.com", nil
}

func (c *recCloud) DescribeProject(context.Context, string) (usecase.CloudProject, bool, error) {
	c.calls = append(c.calls, "DescribeProject")
	return usecase.CloudProject{}, false, nil
}

func (c *recCloud) CreateProject(context.Context, string) error {
	c.calls = append(c.calls, "CreateProject")
	return nil
}

func (c *recCloud) EnableYouTube(context.Context, string) error {
	c.calls = append(c.calls, "EnableYouTube")
	return nil
}

// setupAuth は、取り込みとログインの流れを真似る偽の Authenticator。
type setupAuth struct {
	stubAuth
	imported  bool
	loginRead string // Login が Input から読んだ1行
}

func (a *setupAuth) ClientSecretProjectID(string) (string, error) { return "", nil }
func (a *setupAuth) ImportClientSecret(string) error              { a.imported = true; return nil }
func (a *setupAuth) Inspect() (usecase.AuthInspection, error) {
	return usecase.AuthInspection{KeyStore: "os", ClientSecret: a.imported}, nil
}

func (a *setupAuth) Login(_ context.Context, opt usecase.LoginOptions) error {
	line, err := bufio.NewReader(opt.Input).ReadString('\n')
	a.loginRead = strings.TrimSpace(line)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	a.token = true
	return nil
}

func setupDeps(t *testing.T, stdin string) (Deps, *bytes.Buffer, *bytes.Buffer, *recCloud, *setupAuth) {
	t.Helper()
	d, out, errb, _ := newDeps(t)
	d.Stdin = strings.NewReader(stdin)
	d.Interactive = true
	cloud := &recCloud{}
	auth := &setupAuth{keyStore: "os"}
	r := stubRemote()
	d.Setup = setup.Setup{
		Init:   d.Init,
		Policy: d.Policy,
		Auth:   auth,
		Cloud:  cloud,
		Login:  account.Login{Auth: auth, Remote: r},
		Status: account.Status{Dir: d.Init.Dir, Auth: auth, Remote: r},
	}
	return d, out, errb, cloud, auth
}

func TestSetupCommand(t *testing.T) {
	t.Run("端末でなければ終了コード 2 で、gcloud を呼ばない", func(t *testing.T) {
		// Arrange
		d, _, errb, cloud, _ := setupDeps(t, "")
		d.Interactive = false

		// Act
		code := Execute(context.Background(), d, []string{"setup"})

		// Assert
		if code != 2 || !strings.Contains(errb.String(), "人が端末で実行してください") {
			t.Errorf("code=%d err=%q", code, errb.String())
		}
		if len(cloud.calls) != 0 {
			t.Errorf("gcloud が呼ばれた: %v", cloud.calls)
		}
	})

	t.Run("--json を付けると終了コード 2 で、gcloud を呼ばない", func(t *testing.T) {
		// Arrange
		d, _, errb, cloud, _ := setupDeps(t, "")

		// Act
		code := Execute(context.Background(), d, []string{"--json", "setup"})

		// Assert
		if code != 2 || !strings.Contains(errb.String(), "--json に対応していません") {
			t.Errorf("code=%d err=%q", code, errb.String())
		}
		if len(cloud.calls) != 0 {
			t.Errorf("gcloud が呼ばれた: %v", cloud.calls)
		}
	})

	t.Run("拒否リストのファイルがなくても、入口では止めない", func(t *testing.T) {
		// Arrange（init していないので policy.toml も設定ディレクトリもない）
		d, _, errb, cloud, _ := setupDeps(t, "")
		d.Interactive = false

		// Act
		code := Execute(context.Background(), d, []string{"setup"})

		// Assert（root の判定で止まらず、setup 自身の判定に届いている）
		if code != 2 || !strings.Contains(errb.String(), "人が端末で実行してください") {
			t.Errorf("code=%d err=%q", code, errb.String())
		}
		if len(cloud.calls) != 0 {
			t.Errorf("gcloud が呼ばれた: %v", cloud.calls)
		}
	})

	t.Run("結果を標準出力にまとめて出す", func(t *testing.T) {
		// Arrange
		d, out, _, _, _ := setupDeps(t, "my-proj-12345\n/tmp/client.json\ncode-line\n")

		// Act
		code := Execute(context.Background(), d, []string{"setup", "--no-browser"})

		// Assert
		want := "プロジェクト: my-proj-12345（gcloud のアカウント: me@example.com）\n" +
			"ログイン: テストチャンネル\n" +
			"チャンネル: https://www.youtube.com/channel/UCtest\n" +
			"鍵の置き場所: OS のキーチェーン\n" +
			"ダウンロードした JSON は、パスワードマネージャーなど安全な場所に保管してください。\n"
		if code != 0 || out.String() != want {
			t.Errorf("code=%d out=%q", code, out.String())
		}
	})

	t.Run("質問とログインが同じ読み手を使う", func(t *testing.T) {
		// Arrange
		d, _, _, _, auth := setupDeps(t, "my-proj-12345\n/tmp/client.json\ncode-line\n")

		// Act
		code := Execute(context.Background(), d, []string{"setup", "--no-browser"})

		// Assert
		if code != 0 || auth.loginRead != "code-line" {
			t.Errorf("code=%d loginRead=%q", code, auth.loginRead)
		}
	})

	t.Run("setup --help が使い方を出す", func(t *testing.T) {
		// Arrange
		d, out, _, cloud, _ := setupDeps(t, "")

		// Act
		code := Execute(context.Background(), d, []string{"setup", "--help"})

		// Assert
		if code != 0 || !strings.Contains(out.String(), "--no-browser") || !strings.Contains(out.String(), "--project") {
			t.Errorf("code=%d out=%q", code, out.String())
		}
		if len(cloud.calls) != 0 {
			t.Errorf("gcloud が呼ばれた: %v", cloud.calls)
		}
	})
}

func TestTermPrompter(t *testing.T) {
	t.Run("Confirm は y と yes だけを true にする", func(t *testing.T) {
		for _, tc := range []struct {
			in   string
			want bool
		}{{"y\n", true}, {"yes\n", true}, {"YES\r\n", true}, {"Y\n", true}, {"\n", false}, {"n\n", false}, {"yep\n", false}} {
			// Arrange
			var errb bytes.Buffer
			p := termPrompter{in: bufio.NewReader(strings.NewReader(tc.in)), out: &errb}

			// Act
			got, err := p.Confirm("続けますか")

			// Assert
			if err != nil || got != tc.want {
				t.Errorf("in=%q got=%v err=%v", tc.in, got, err)
			}
			if !strings.Contains(errb.String(), "続けますか [y/N]: ") {
				t.Errorf("質問の表示: %q", errb.String())
			}
		}
	})

	t.Run("Ask は Enter だけなら既定値を返し、入力の終わりは io.EOF", func(t *testing.T) {
		// Arrange
		var errb bytes.Buffer
		p := termPrompter{in: bufio.NewReader(strings.NewReader("\nabc\r\n")), out: &errb}

		// Act
		a, errA := p.Ask("ID", "def")
		b, errB := p.Ask("ID", "def")
		_, errC := p.Ask("ID", "def")

		// Assert
		if errA != nil || a != "def" || errB != nil || b != "abc" || !errors.Is(errC, io.EOF) {
			t.Errorf("a=%q,%v b=%q,%v c=%v", a, errA, b, errB, errC)
		}
		if !strings.Contains(errb.String(), "ID [def]: ") {
			t.Errorf("質問の表示: %q", errb.String())
		}
	})

	for _, method := range []string{"Ask", "Confirm"} {
		t.Run(method+" は、入力が届かないまま ctx が取り消されたら、すぐに「中止しました」の KindUsage を返す", func(t *testing.T) {
			// Arrange
			pr, pw := io.Pipe() // 何も書かない
			t.Cleanup(func() { _ = pw.Close() })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out := newWatchWriter("[")
			p := termPrompter{ctx: ctx, in: bufio.NewReader(pr), out: out}
			done := make(chan error, 1)

			// Act
			go func() {
				var err error
				if method == "Ask" {
					_, err = p.Ask("ID", "def")
				} else {
					_, err = p.Confirm("続けますか")
				}
				done <- err
			}()
			<-out.seen // 質問を出した（読み始める）あとで取り消す
			cancel()

			// Assert
			select {
			case err := <-done:
				if domain.KindOf(err) != domain.KindUsage || err.Error() != "中止しました" {
					t.Errorf("err = %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("取り消しても戻らない")
			}
		})
	}

	t.Run("ctx が取り消し済みなら、読み始めずに「中止しました」を返す", func(t *testing.T) {
		// Arrange
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		br := bufio.NewReader(strings.NewReader("abc\n"))
		var errb bytes.Buffer
		p := termPrompter{ctx: ctx, in: br, out: &errb}

		// Act
		_, err := p.Ask("ID", "def")

		// Assert
		if domain.KindOf(err) != domain.KindUsage || err.Error() != "中止しました" {
			t.Errorf("err = %v", err)
		}
		if rest, _ := br.ReadString('\n'); rest != "abc\n" {
			t.Errorf("読んでしまった: 残り=%q", rest)
		}
	})
}

// watchWriter は、書かれたものを貯め、want を含む書き込みが来たら seen を閉じる。
type watchWriter struct {
	mu   sync.Mutex
	buf  strings.Builder
	want string
	seen chan struct{}
	once sync.Once
}

func newWatchWriter(want string) *watchWriter {
	return &watchWriter{want: want, seen: make(chan struct{})}
}

func (w *watchWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if strings.Contains(w.buf.String(), w.want) {
		w.once.Do(func() { close(w.seen) })
	}
	return len(p), nil
}

func (w *watchWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func TestSetupCommandCancel(t *testing.T) {
	t.Run("質問の途中で Ctrl-C（ctx の取り消し）が来たら、次の行を待たずに終了コード 2 の「中止しました」で止まる", func(t *testing.T) {
		// Arrange
		d, _, _, cloud, _ := setupDeps(t, "")
		pr, pw := io.Pipe() // 何も書かない
		t.Cleanup(func() { _ = pw.Close() })
		d.Stdin = pr
		errw := newWatchWriter("プロジェクト ID")
		d.Stderr = errw
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		codeCh := make(chan int, 1)

		// Act
		go func() { codeCh <- Execute(ctx, d, []string{"setup"}) }()
		<-errw.seen
		cancel()

		// Assert
		select {
		case code := <-codeCh:
			if code != 2 || !strings.Contains(errw.String(), "中止しました") || strings.Contains(errw.String(), "gcloud projects") {
				t.Errorf("code=%d err=%q", code, errw.String())
			}
			if strings.Contains(strings.Join(cloud.calls, ","), "Project") {
				t.Errorf("gcloud のプロジェクト操作が呼ばれた: %v", cloud.calls)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("取り消しても戻らない")
		}
	})
}
