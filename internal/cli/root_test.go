package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/repository/config"
	"github.com/wadoyoka/youtube-cli/internal/usecase/setup"
)

func newDeps(t *testing.T) (Deps, *bytes.Buffer, *bytes.Buffer, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cfg")
	cd := config.NewDir(dir)
	pf := config.NewPolicyFile(dir)
	var out, errb bytes.Buffer
	d := Deps{
		Init:   setup.Init{Dir: cd, Policy: pf},
		Policy: pf,
		Stdin:  strings.NewReader(""),
		Stdout: &out,
		Stderr: &errb,
	}
	return d, &out, &errb, dir
}

func denyLogout(t *testing.T, d Deps, dir string, errb *bytes.Buffer) {
	t.Helper()
	Execute(context.Background(), d, []string{"init"})
	if err := os.WriteFile(filepath.Join(dir, "policy.toml"), []byte(`deny = ["logout"]`), 0o600); err != nil {
		t.Fatal(err)
	}
	errb.Reset()
}

func TestExecute(t *testing.T) {
	t.Run("init は 0 を返し作成しましたと表示する", func(t *testing.T) {
		// Arrange
		d, out, _, _ := newDeps(t)

		// Act
		code := Execute(context.Background(), d, []string{"init"})

		// Assert
		if code != 0 || !strings.Contains(out.String(), "作成しました") {
			t.Errorf("code=%d out=%q", code, out.String())
		}
	})

	t.Run("--json init は JSON を1つ出す", func(t *testing.T) {
		// Arrange
		d, out, _, _ := newDeps(t)

		// Act
		code := Execute(context.Background(), d, []string{"--json", "init"})

		// Assert
		var got map[string]any
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("json: %v: %q", err, out.String())
		}
		if code != 0 || got["policyCreated"] != true {
			t.Errorf("code=%d got=%v", code, got)
		}
	})

	t.Run("拒否リストにあるコマンドは 7 で止まる", func(t *testing.T) {
		// Arrange
		d, _, errb, dir := newDeps(t)
		denyLogout(t, d, dir, errb)

		// Act
		code := Execute(context.Background(), d, []string{"logout"})

		// Assert
		if code != 7 || !strings.Contains(errb.String(), "拒否") {
			t.Errorf("code=%d err=%q", code, errb.String())
		}
	})

	t.Run("policy.toml がなければ 3", func(t *testing.T) {
		// Arrange
		d, _, _, _ := newDeps(t)

		// Act
		code := Execute(context.Background(), d, []string{"logout"})

		// Assert
		if code != 3 {
			t.Errorf("code=%d", code)
		}
	})

	t.Run("--json を付けるとエラーも JSON になる", func(t *testing.T) {
		// Arrange
		d, _, errb, dir := newDeps(t)
		denyLogout(t, d, dir, errb)

		// Act
		code := Execute(context.Background(), d, []string{"--json", "logout"})

		// Assert
		if code != 7 || !strings.Contains(errb.String(), `"kind":"denied"`) {
			t.Errorf("code=%d err=%q", code, errb.String())
		}
	})

	t.Run("知らないフラグは 2", func(t *testing.T) {
		// Arrange
		d, _, _, _ := newDeps(t)

		// Act
		code := Execute(context.Background(), d, []string{"--nope"})

		// Assert
		if code != 2 {
			t.Errorf("code=%d", code)
		}
	})

	t.Run("知らないサブコマンドは 2", func(t *testing.T) {
		// Arrange
		d, _, _, _ := newDeps(t)

		// Act
		code := Execute(context.Background(), d, []string{"nope"})

		// Assert
		if code != 2 {
			t.Errorf("code=%d", code)
		}
	})

	t.Run("init に余計な引数を渡すと 2", func(t *testing.T) {
		// Arrange
		d, _, _, _ := newDeps(t)

		// Act
		code := Execute(context.Background(), d, []string{"init", "x"})

		// Assert
		if code != 2 {
			t.Errorf("code=%d", code)
		}
	})
}
