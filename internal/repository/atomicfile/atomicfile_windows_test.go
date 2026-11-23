//go:build windows

package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteWindows(t *testing.T) {
	t.Run("ほかのプロセスが開いている間の置き換えは、閉じたあとに試し直して成功する", func(t *testing.T) {
		// Arrange
		path := filepath.Join(t.TempDir(), "target.json")
		if err := Write(path, []byte("first"), 0o600); err != nil {
			t.Fatal(err)
		}
		held, err := os.Open(path) //nolint:gosec // テストが作った一時ファイル
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = held.Close() }()
		type result struct {
			err     error
			elapsed time.Duration
		}
		done := make(chan result, 1)

		// Act
		go func() {
			start := time.Now()
			err := Write(path, []byte("second"), 0o600)
			done <- result{err: err, elapsed: time.Since(start)}
		}()
		time.Sleep(200 * time.Millisecond)
		if err := held.Close(); err != nil {
			t.Fatal(err)
		}
		var got result
		select {
		case got = <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Write が終わりません")
		}

		// Assert
		if got.err != nil {
			t.Fatalf("Write: %v", got.err)
		}
		if got.elapsed < 150*time.Millisecond {
			t.Errorf("elapsed = %v, want >= 150ms（試し直した証拠）", got.elapsed)
		}
		data, err := os.ReadFile(path) //nolint:gosec // テストが作った一時ファイル
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "second" {
			t.Errorf("data = %q, want %q", data, "second")
		}
	})
}
