package gcloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

// 偽物の gcloud は、テストの実行ファイル自身。包みのファイル（sh か cmd.exe を通る）から呼ばれる。
func TestMain(m *testing.M) {
	if os.Getenv("YOUTUBE_CLI_FAKE_GCLOUD") == "1" {
		os.Exit(runFakeGCloud())
	}
	os.Exit(m.Run())
}

func runFakeGCloud() int {
	args := strings.Join(os.Args[1:], "\n") + "\n"
	if len(os.Args) == 1 {
		args = "\n"
	}
	if err := os.WriteFile(os.Getenv("FAKE_GCLOUD_ARGS"), []byte(args), 0o600); err != nil { //nolint:gosec // テストが渡した出力先
		return 70
	}
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 70
	}
	if err := os.WriteFile(os.Getenv("FAKE_GCLOUD_STDIN"), in, 0o600); err != nil { //nolint:gosec // テストが渡した出力先
		return 70
	}
	_, _ = fmt.Fprint(os.Stdout, os.Getenv("FAKE_GCLOUD_STDOUT"))
	_, _ = fmt.Fprint(os.Stderr, os.Getenv("FAKE_GCLOUD_STDERR"))
	if v := os.Getenv("FAKE_GCLOUD_EXIT"); v != "" {
		code, cerr := strconv.Atoi(v)
		if cerr != nil {
			return 70
		}
		return code
	}
	return 0
}

// writeGCloudWrapper は、偽物を呼ぶ包みのファイルを dir に置き、そのパスを返す。
// 本物の gcloud と同じく、sh（Windows では cmd.exe）を通して引数が渡る。
func writeGCloudWrapper(t *testing.T, dir string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_GCLOUD_HELPER", exe)
	t.Setenv("YOUTUBE_CLI_FAKE_GCLOUD", "1")
	name, body, mode := "gcloud", "#!/bin/sh\nexec \"$FAKE_GCLOUD_HELPER\" \"$@\"\n", os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		name, body, mode = "gcloud.cmd", "@\"%FAKE_GCLOUD_HELPER%\" %*", 0o644
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

type fakeGCloud struct {
	dir       string
	argsFile  string
	stdinFile string
}

// wrapperDir は、包みのファイルを置くディレクトリを作る。t.TempDir() はテストの名前（= を含みうる）から
// パスを作り、cmd.exe は引用符のないコマンド名を = で切るので、名前に左右されない場所に作る。
func wrapperDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "fake-gcloud-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func newFakeGCloud(t *testing.T, stdout, stderr string, exit string) (*GCloud, *fakeGCloud) {
	t.Helper()
	dir := t.TempDir()
	wdir := wrapperDir(t)
	bin := writeGCloudWrapper(t, wdir)
	f := &fakeGCloud{dir: wdir, argsFile: filepath.Join(dir, "args.txt"), stdinFile: filepath.Join(dir, "stdin.txt")}
	t.Setenv("FAKE_GCLOUD_ARGS", f.argsFile)
	t.Setenv("FAKE_GCLOUD_STDIN", f.stdinFile)
	t.Setenv("FAKE_GCLOUD_STDOUT", stdout)
	t.Setenv("FAKE_GCLOUD_STDERR", stderr)
	t.Setenv("FAKE_GCLOUD_EXIT", exit)
	return &GCloud{Bin: bin}, f
}

func (f *fakeGCloud) args(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(f.argsFile)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func TestGCloud(t *testing.T) {
	ctx := context.Background()

	t.Run("JSON の最初の account を返す", func(t *testing.T) {
		// Arrange
		g, f := newFakeGCloud(t, `[{"account":"me@example.com","status":"ACTIVE"},{"account":"other@example.com","status":"ACTIVE"}]`, "", "0")
		// Act
		got, err := g.ActiveAccount(ctx)
		// Assert
		if err != nil || got != "me@example.com" {
			t.Fatalf("got %q, %v", got, err)
		}
		want := []string{"auth", "list", "--filter=status:ACTIVE", "--format=json", "--quiet"}
		if a := f.args(t); strings.Join(a, "|") != strings.Join(want, "|") {
			t.Errorf("args = %v, want %v", a, want)
		}
	})

	t.Run("ログインしていなければ空文字", func(t *testing.T) {
		// Arrange
		g, _ := newFakeGCloud(t, "[]", "", "0")
		// Act
		got, err := g.ActiveAccount(ctx)
		// Assert
		if err != nil || got != "" {
			t.Fatalf("got %q, %v", got, err)
		}
	})

	t.Run("出力が空や空白だけならログインしていない（空文字）", func(t *testing.T) {
		// Arrange
		g, _ := newFakeGCloud(t, "  \n", "", "0")
		// Act
		got, err := g.ActiveAccount(ctx)
		// Assert
		if err != nil || got != "" {
			t.Fatalf("got %q, %v", got, err)
		}
	})

	t.Run("JSON でなければエラー", func(t *testing.T) {
		// Arrange
		g, _ := newFakeGCloud(t, "me@example.com", "", "0")
		// Act
		_, err := g.ActiveAccount(ctx)
		// Assert
		if domain.KindOf(err) != domain.KindUnexpected || err == nil {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("describe: JSON から ID と状態を読む", func(t *testing.T) {
		// Arrange
		g, _ := newFakeGCloud(t, `{"projectId":"proj-abc123","lifecycleState":"ACTIVE"}`, "", "0")
		// Act
		p, found, err := g.DescribeProject(ctx, "proj-abc123")
		// Assert
		if err != nil || !found || p.ID != "proj-abc123" || p.State != "ACTIVE" {
			t.Fatalf("p = %+v, found = %v, err = %v", p, found, err)
		}
	})

	t.Run("describe: PERMISSION_DENIED なら found=false", func(t *testing.T) {
		// Arrange
		g, _ := newFakeGCloud(t, "", "ERROR: PERMISSION_DENIED: nope", "1")
		// Act
		_, found, err := g.DescribeProject(ctx, "proj-abc123")
		// Assert
		if err != nil || found {
			t.Fatalf("found = %v, err = %v", found, err)
		}
	})

	t.Run("describe: そのほかの失敗はエラーで、詳細に標準エラーが入る", func(t *testing.T) {
		// Arrange
		g, _ := newFakeGCloud(t, "", "boom happened", "1")
		// Act
		_, _, err := g.DescribeProject(ctx, "proj-abc123")
		// Assert
		var de *domain.Error
		if err == nil || !asDomainError(err, &de) {
			t.Fatalf("err = %v", err)
		}
		if de.Kind != domain.KindUnexpected || !strings.Contains(strings.Join(de.Details, "\n"), "boom happened") {
			t.Errorf("kind = %v, details = %v", de.Kind, de.Details)
		}
	})

	t.Run("create: 表示名 youtube-cli を渡す", func(t *testing.T) {
		// Arrange
		g, f := newFakeGCloud(t, "", "", "0")
		// Act
		err := g.CreateProject(ctx, "proj-abc123")
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		want := "projects|create|proj-abc123|--name=youtube-cli|--quiet"
		if got := strings.Join(f.args(t), "|"); got != want {
			t.Errorf("args = %q, want %q", got, want)
		}
	})

	t.Run("create: already exists なら KindConflict", func(t *testing.T) {
		// Arrange
		g, _ := newFakeGCloud(t, "", "ERROR: Project already exists", "1")
		// Act
		err := g.CreateProject(ctx, "proj-abc123")
		// Assert
		if domain.KindOf(err) != domain.KindConflict || err == nil {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("enable: youtube.googleapis.com と --project を渡す", func(t *testing.T) {
		// Arrange
		g, f := newFakeGCloud(t, "", "", "0")
		// Act
		err := g.EnableYouTube(ctx, "proj-abc123")
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		want := "services|enable|youtube.googleapis.com|--project=proj-abc123|--quiet"
		if got := strings.Join(f.args(t), "|"); got != want {
			t.Errorf("args = %q, want %q", got, want)
		}
	})

	t.Run("毎回 --quiet を付ける", func(t *testing.T) {
		// Arrange
		g, f := newFakeGCloud(t, "[]", "", "0")
		// Act
		_, err := g.ActiveAccount(ctx)
		// Assert
		a := f.args(t)
		if err != nil || a[len(a)-1] != "--quiet" {
			t.Errorf("args = %v, err = %v", a, err)
		}
	})

	t.Run("標準入力をつながない", func(t *testing.T) {
		// Arrange
		g, f := newFakeGCloud(t, "", "", "0")
		tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		// Act
		err := g.EnableYouTube(tctx, "proj-abc123")
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		b, rerr := os.ReadFile(f.stdinFile)
		if rerr != nil || len(b) != 0 {
			t.Errorf("stdin = %q, err = %v", b, rerr)
		}
	})

	t.Run("許可していない呼び出しは起動しない", func(t *testing.T) {
		// Arrange
		deny := [][]string{
			{"projects", "delete", "proj-abc123"},
			{"services", "disable", "youtube.googleapis.com", "--project=proj-abc123"},
			{"auth", "login"},
			{"billing", "accounts", "list"},
			{"projects", "describe", "--impersonate-service-account=x", "--format=json"},
			{"projects", "describe", "proj-abc123", "--format=json", "--impersonate-service-account=x"},
			{"projects", "create", "proj-abc123", "--name=youtube-cli", "--extra"},
			{"projects", "create", "BAD_ID", "--name=youtube-cli"},
			{"services", "enable", "youtube.googleapis.com", "--project=BAD_ID"},
			{},
		}
		allow := [][]string{
			{"auth", "list", "--filter=status:ACTIVE", "--format=json"},
			{"projects", "describe", "proj-abc123", "--format=json"},
			{"projects", "create", "proj-abc123", "--name=youtube-cli"},
			{"services", "enable", "youtube.googleapis.com", "--project=proj-abc123"},
		}
		// Act / Assert
		for _, a := range deny {
			if allowedGCloud(a) {
				t.Errorf("allowed %v", a)
			}
		}
		for _, a := range allow {
			if !allowedGCloud(a) {
				t.Errorf("denied %v", a)
			}
		}
		g, f := newFakeGCloud(t, "", "", "0")
		_, _, err := g.run(ctx, "projects", "delete", "proj-abc123")
		if domain.KindOf(err) != domain.KindUnexpected || err == nil {
			t.Errorf("err = %v", err)
		}
		if _, serr := os.Stat(f.argsFile); serr == nil {
			t.Error("gcloud が起動した")
		}
	})

	t.Run("Bin が空なら PATH から探す", func(t *testing.T) {
		// Arrange
		_, f := newFakeGCloud(t, `[{"account":"x@example.com"}]`, "", "0")
		t.Setenv("PATH", f.dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		g := NewGCloud()
		// Act
		got, err := g.ActiveAccount(ctx)
		// Assert
		if err != nil || got != "x@example.com" || !g.Installed() {
			t.Errorf("got %q, err = %v, installed = %v", got, err, g.Installed())
		}
	})

	t.Run("見つからなければ Installed は false", func(t *testing.T) {
		// Arrange
		t.Setenv("PATH", t.TempDir())
		g := NewGCloud()
		// Act
		got := g.Installed()
		// Assert
		if got {
			t.Error("Installed = true")
		}
	})
}

func asDomainError(err error, target **domain.Error) bool {
	return errors.As(err, target)
}

func TestGCloudキャンセル(t *testing.T) {
	t.Run("孫プロセスが出力を握ったままでもキャンセルで返る", func(t *testing.T) {
		// Arrange
		if runtime.GOOS == "windows" {
			t.Skip("Windows では孫プロセスの作り方が違うので試さない")
		}
		dir := wrapperDir(t)
		script := "#!/bin/sh\n/bin/sleep 20 &\n/bin/sleep 20\n"
		bin := filepath.Join(dir, "gcloud")
		if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // テスト用の実行ファイル
			t.Fatal(err)
		}
		g := &GCloud{Bin: bin}
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		// Act
		start := time.Now()
		_, err := g.ActiveAccount(ctx)
		elapsed := time.Since(start)
		// Assert
		if err == nil {
			t.Error("エラーになるはず")
		}
		if elapsed > 10*time.Second {
			t.Errorf("戻るまで %v かかった", elapsed)
		}
	})
}
