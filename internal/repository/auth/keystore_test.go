package auth

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

type fakeKeyring struct {
	items   map[string]string
	getErr  error
	setErr  error
	lastGet string
}

func newFakeKeyring() *fakeKeyring { return &fakeKeyring{items: map[string]string{}} }

func (f *fakeKeyring) Get(service, user string) (string, error) {
	f.lastGet = user
	if f.getErr != nil {
		return "", f.getErr
	}
	v, ok := f.items[service+"|"+user]
	if !ok {
		return "", errKeyringNotFound
	}
	return v, nil
}

func (f *fakeKeyring) Set(service, user, secret string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.items[service+"|"+user] = secret
	return nil
}

func TestKeyStore(t *testing.T) {
	t.Run("OS: 項目がなければ found=false でエラーなし", func(t *testing.T) {
		// Arrange
		s := newOSKeyStore(t.TempDir(), newFakeKeyring())
		// Act
		got, found, err := s.Load()
		// Assert
		if err != nil || found || got.Key != nil {
			t.Fatalf("key=%v found=%v err=%v", got.Key, found, err)
		}
	})

	t.Run("OS: 読み出しが失敗したら KindSetup で、YOUTUBE_CLI_KEYRING=file の案内を含む", func(t *testing.T) {
		// Arrange
		b := newFakeKeyring()
		b.getErr = errors.New("dbus がありません")
		s := newOSKeyStore(t.TempDir(), b)
		// Act
		_, _, err := s.Load()
		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Fatalf("kind = %v", domain.KindOf(err))
		}
		if !strings.Contains(err.Error(), "YOUTUBE_CLI_KEYRING=file") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("OS: アカウント名は設定ディレクトリの絶対パス", func(t *testing.T) {
		// Arrange
		b := newFakeKeyring()
		s := newOSKeyStore("rel-dir", b)
		want, _ := filepath.Abs("rel-dir")
		// Act
		_, _, _ = s.Load()
		// Assert
		if b.lastGet != want || !filepath.IsAbs(b.lastGet) {
			t.Fatalf("user = %q", b.lastGet)
		}
	})

	t.Run("OS: 保存した鍵を読み戻せる", func(t *testing.T) {
		// Arrange
		s := newOSKeyStore(t.TempDir(), newFakeKeyring())
		k, _ := newKey()
		// Act
		saveErr := s.Save(k)
		got, found, err := s.Load()
		// Assert
		if saveErr != nil || err != nil || !found || string(got.Key) != string(k) {
			t.Fatalf("save=%v err=%v found=%v", saveErr, err, found)
		}
	})

	t.Run("OS: 中身が base64 の 32 バイトでなければ KindSetup", func(t *testing.T) {
		for _, v := range []string{"%%%not-base64", base64.StdEncoding.EncodeToString([]byte("short"))} {
			// Arrange
			dir := t.TempDir()
			b := newFakeKeyring()
			abs, _ := filepath.Abs(dir)
			b.items[keyringService+"|"+abs] = v
			s := newOSKeyStore(dir, b)
			// Act
			_, _, err := s.Load()
			// Assert
			if domain.KindOf(err) != domain.KindSetup {
				t.Fatalf("%q: kind = %v", v, domain.KindOf(err))
			}
		}
	})

	t.Run("file: なければ found=false", func(t *testing.T) {
		// Arrange
		s := newFileKeyStore(t.TempDir())
		// Act
		got, found, err := s.Load()
		// Assert
		if err != nil || found || got.Key != nil {
			t.Fatalf("key=%v found=%v err=%v", got.Key, found, err)
		}
	})

	t.Run("file: 保存すると権限 0600 で、読み戻せる", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		s := newFileKeyStore(dir)
		k, _ := newKey()
		// Act
		saveErr := s.Save(k)
		got, found, err := s.Load()
		info, statErr := os.Stat(filepath.Join(dir, "key"))
		// Assert
		if saveErr != nil || err != nil || statErr != nil || !found || string(got.Key) != string(k) {
			t.Fatalf("save=%v err=%v stat=%v found=%v", saveErr, err, statErr, found)
		}
		t.Run("権限は 0600", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows ではファイルの権限の数字が効かない")
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("perm = %v", info.Mode().Perm())
			}
		})
	})

	t.Run("file: 中身が壊れていれば KindSetup で、ファイルは書き換わらない", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "key")
		if err := os.WriteFile(path, []byte("broken"), 0o600); err != nil {
			t.Fatal(err)
		}
		s := newFileKeyStore(dir)
		// Act
		_, _, err := s.Load()
		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Fatalf("kind = %v", domain.KindOf(err))
		}
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("err = %v", err)
		}
		got, _ := os.ReadFile(path) //nolint:gosec // テストの一時ファイル
		if string(got) != "broken" {
			t.Fatalf("書き換わった: %q", got)
		}
	})

	t.Run("file: 読む権限がなければ KindSetup", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root では権限が効かない")
		}
		// Arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "key")
		if err := os.WriteFile(path, []byte("x"), 0o000); err != nil {
			t.Fatal(err)
		}
		s := newFileKeyStore(dir)
		// Act
		_, _, err := s.Load()
		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Fatalf("kind = %v", domain.KindOf(err))
		}
	})

	t.Run("不正な YOUTUBE_CLI_KEYRING は KindUsage", func(t *testing.T) {
		// Arrange
		s := NewKeyStore(t.TempDir(), "bogus")
		// Act
		_, _, loadErr := s.Load()
		saveErr := s.Save(make([]byte, 32))
		// Assert
		if domain.KindOf(loadErr) != domain.KindUsage || domain.KindOf(saveErr) != domain.KindUsage {
			t.Fatalf("load=%v save=%v", loadErr, saveErr)
		}
		if s.Kind() != "bogus" || !strings.Contains(loadErr.Error(), "bogus") {
			t.Fatalf("kind=%q err=%v", s.Kind(), loadErr)
		}
	})

	t.Run("go-keyring・file・不正な値の LoadLocked は Load と同じ結果を返す", func(t *testing.T) {
		// Arrange
		k, _ := newKey()
		dir := t.TempDir()
		osStore, fileStore := newOSKeyStore(dir, newFakeKeyring()), newFileKeyStore(dir)
		for _, s := range []KeyStore{osStore, fileStore} {
			if err := s.Save(k); err != nil {
				t.Fatal(err)
			}
		}
		for _, s := range []KeyStore{osStore, fileStore, invalidKeyStore{mode: "bogus"}} {
			// Act
			want, wantFound, wantErr := s.Load()
			got, gotFound, gotErr := s.LoadLocked()
			// Assert
			if string(got.Key) != string(want.Key) || got.Unverified != want.Unverified || gotFound != wantFound ||
				domain.KindOf(gotErr) != domain.KindOf(wantErr) || (gotErr == nil) != (wantErr == nil) {
				t.Fatalf("%s: LoadLocked=(%v, %v) Load=(%v, %v)", s.Kind(), gotFound, gotErr, wantFound, wantErr)
			}
		}
	})

	t.Run("空文字と os は OS、file はファイルになる", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		// Act
		a, b, c := NewKeyStore(dir, "").Kind(), NewKeyStore(dir, "os").Kind(), NewKeyStore(dir, "file").Kind()
		// Assert
		if a != "os" || b != "os" || c != "file" {
			t.Fatalf("%q %q %q", a, b, c)
		}
	})

	t.Run("エラーの文言に鍵が含まれない", func(t *testing.T) {
		// Arrange
		k, _ := newKey()
		enc := base64.StdEncoding.EncodeToString(k)
		dir := t.TempDir()
		fb := newFakeKeyring()
		fb.setErr = errors.New("拒否: " + "failed")
		var msgs []string
		// 壊れた値（鍵に似た長さ違いの値）
		abs, _ := filepath.Abs(dir)
		fb.items[keyringService+"|"+abs] = enc + "AAAA"
		_, _, err := newOSKeyStore(dir, fb).Load()
		msgs = append(msgs, err.Error())
		_ = os.WriteFile(filepath.Join(dir, "key"), []byte(enc+"AAAA"), 0o600)
		_, _, err = newFileKeyStore(dir).Load()
		msgs = append(msgs, err.Error())
		// Act
		err = newOSKeyStore(dir, fb).Save(k)
		msgs = append(msgs, err.Error())
		err = newFileKeyStore(filepath.Join(dir, "key", "nodir")).Save(k)
		msgs = append(msgs, err.Error())
		// Assert
		for _, m := range msgs {
			if strings.Contains(m, enc) || strings.Contains(m, enc+"AAAA") {
				t.Fatalf("鍵が含まれる: %s", m)
			}
		}
	})
}
