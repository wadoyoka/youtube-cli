package auth

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSnapshotToken(t *testing.T) {
	t.Run("token.enc があれば、書き換えたあとで restore すると元の中身に戻る", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		a := &Auth{Dir: dir, Keys: newFileKeyStore(dir)}
		path := filepath.Join(dir, tokenName)
		old := []byte("old-sealed-token")
		if err := os.WriteFile(path, old, 0o600); err != nil {
			t.Fatal(err)
		}
		restore, err := a.SnapshotToken()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("new-sealed-token"), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		err = restore()

		// Assert
		got, readErr := os.ReadFile(path) //nolint:gosec // テストの一時ディレクトリ
		if err != nil || readErr != nil || !bytes.Equal(got, old) {
			t.Fatalf("err=%v readErr=%v got=%q", err, readErr, got)
		}
		if runtime.GOOS != "windows" {
			info, _ := os.Stat(path)
			if info.Mode().Perm() != 0o600 {
				t.Errorf("perm = %v", info.Mode().Perm())
			}
		}
	})

	t.Run("token.enc がなかったなら、restore すると消える", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		a := &Auth{Dir: dir, Keys: newFileKeyStore(dir)}
		path := filepath.Join(dir, tokenName)
		restore, err := a.SnapshotToken()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("new-sealed-token"), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		err = restore()

		// Assert
		if _, statErr := os.Stat(path); err != nil || !errors.Is(statErr, fs.ErrNotExist) {
			t.Errorf("err=%v statErr=%v", err, statErr)
		}
	})

	t.Run("なかったままでも restore は失敗しない", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		a := &Auth{Dir: dir, Keys: newFileKeyStore(dir)}
		restore, err := a.SnapshotToken()
		if err != nil {
			t.Fatal(err)
		}

		// Act
		err = restore()
		// Assert
		if err != nil {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("token.enc を読めないときはエラー", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		a := &Auth{Dir: dir, Keys: newFileKeyStore(dir)}
		if err := os.Mkdir(filepath.Join(dir, tokenName), 0o700); err != nil { // ファイルではなくディレクトリ
			t.Fatal(err)
		}

		// Act
		_, err := a.SnapshotToken()

		// Assert
		if err == nil {
			t.Error("エラーになるはず")
		}
	})
}
