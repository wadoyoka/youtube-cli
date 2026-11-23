package localfile_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/repository/localfile"
)

func TestOpen(t *testing.T) {
	t.Run("ファイルを開き、中身と大きさを返す", func(t *testing.T) {
		// Arrange
		path := filepath.Join(t.TempDir(), "a.mp4")
		if err := os.WriteFile(path, []byte("VIDEO"), 0o600); err != nil {
			t.Fatal(err)
		}

		// Act
		f, size, err := localfile.Open(path)
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		b, _ := io.ReadAll(f)
		if size != 5 || string(b) != "VIDEO" {
			t.Errorf("size=%d body=%q", size, b)
		}
	})

	t.Run("ないパスは fs.ErrNotExist を包んだエラー", func(t *testing.T) {
		// Arrange
		path := filepath.Join(t.TempDir(), "none.mp4")

		// Act
		_, _, err := localfile.Open(path)

		// Assert
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("err=%v", err)
		}
	})

	t.Run("ディレクトリは開かない", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()

		// Act
		f, _, err := localfile.Open(dir)

		// Assert
		if err == nil || f != nil {
			t.Errorf("f=%v err=%v", f, err)
		}
	})
}
