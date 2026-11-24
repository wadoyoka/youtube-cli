package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/domain/policy"
)

func TestPolicyFile(t *testing.T) {
	t.Run("初回は作成して0600、2回目は書き換えない", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		pf := NewPolicyFile(dir)

		// Act
		first, err1 := pf.CreateIfAbsent()
		info, statErr := os.Stat(pf.Path())
		if err := os.WriteFile(pf.Path(), []byte(`deny = ["upload"]`), 0o600); err != nil {
			t.Fatal(err)
		}
		second, err2 := pf.CreateIfAbsent()
		body, _ := os.ReadFile(pf.Path())

		// Assert
		if err1 != nil || err2 != nil || statErr != nil {
			t.Fatalf("errors: %v %v %v", err1, err2, statErr)
		}
		if !first || second {
			t.Errorf("created: first=%v second=%v", first, second)
		}
		t.Run("権限は 0600", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows ではファイルの権限の数字が効かない")
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("perm = %v", info.Mode().Perm())
			}
		})
		if string(body) != `deny = ["upload"]` {
			t.Errorf("body = %q", body)
		}
	})

	t.Run("Load の結果", func(t *testing.T) {
		cases := []struct {
			name     string
			content  *string
			wantKind domain.ErrorKind
			wantErr  bool
			wantDeny []string
		}{
			{"ファイルがなければ準備が必要", nil, domain.KindSetup, true, nil},
			{"既定の内容なら playlist-create と playlist-add を拒否する", new(policy.DefaultTOML), 0, false, []string{"playlist-create", "playlist-add"}},
			{"deny を読める", new(`deny = ["upload"]`), 0, false, []string{"upload"}},
			{"知らないキーは拒否", new(`denny = []`), domain.KindDenied, true, nil},
			{"TOML として壊れていれば拒否", new(`deny = [`), domain.KindDenied, true, nil},
			{"知らないコマンド名は拒否", new(`deny = ["uplaod"]`), domain.KindDenied, true, nil},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				// Arrange
				dir := t.TempDir()
				pf := NewPolicyFile(dir)
				if c.content != nil {
					if err := os.WriteFile(filepath.Join(dir, "policy.toml"), []byte(*c.content), 0o600); err != nil {
						t.Fatal(err)
					}
				}

				// Act
				got, err := pf.Load()

				// Assert
				if c.wantErr {
					if err == nil || domain.KindOf(err) != c.wantKind {
						t.Fatalf("err = %v, want kind %v", err, c.wantKind)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(got.Deny) != len(c.wantDeny) || (len(c.wantDeny) > 0 && !reflect.DeepEqual(got.Deny, c.wantDeny)) {
					t.Errorf("Deny = %v, want %v", got.Deny, c.wantDeny)
				}
			})
		}
	})
}

func TestPolicyFileUnknownKeys(t *testing.T) {
	t.Run("知らないキーは、その名前を日本語のエラーに出す", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		pf := NewPolicyFile(dir)
		if err := os.WriteFile(pf.Path(), []byte("denny = []\nallow = [\"ls\"]\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		_, err := pf.Load()

		// Assert
		if domain.KindOf(err) != domain.KindDenied {
			t.Fatalf("err = %v", err)
		}
		msg := err.Error()
		if !strings.Contains(msg, "denny") || !strings.Contains(msg, "allow") || strings.Contains(msg, "strict mode") {
			t.Errorf("msg = %q", msg)
		}
		var de *domain.Error
		if !errors.As(err, &de) || !reflect.DeepEqual(de.Details, []string{"1 行目: denny", "2 行目: allow"}) {
			t.Errorf("details = %q", de.Details)
		}
	})
}
