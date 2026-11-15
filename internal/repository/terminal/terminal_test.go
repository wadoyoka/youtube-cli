package terminal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsCygwinPipeName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"msys の疑似端末", `\msys-dd50a72ab4668b33-pty2-to-master`, true},
		{"cygwin の疑似端末", `\cygwin-e022582115c10879-pty4-from-master`, true},
		{"NamedPipe 付きの msys", `\Device\NamedPipe\msys-1234-pty0-from-master`, true},
		{"NamedPipe 付きの cygwin", `\Device\NamedPipe\cygwin-1234-pty0-to-master`, true},
		{"1 つ目の区切りだけが違う", `\evil-1234-pty0-to-master`, false},
		{"4 つ目が from でも to でもない", `\msys-1234-pty0-up-master`, false},
		{"名前のないパイプ", `\Device\NamedPipe\`, false},
		{"ふつうのパイプ", `\Device\NamedPipe\random-pipe`, false},
		{"master でなく slave", `\msys-dd50a72ab4668b33-pty2-to-slave`, false},
		{"2 つ目が空", `\msys--pty2-to-master`, false},
		{"pty でなく tty", `\msys-1234-tty2-to-master`, false},
		{"段が足りない", `\cygwin-1234-pty`, false},
		{"空文字", ``, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange

			// Act
			got := isCygwinPipeName(tc.in)

			// Assert
			if got != tc.want {
				t.Errorf("isCygwinPipeName(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	t.Run("ふつうのパイプは端末ではない", func(t *testing.T) {
		// Arrange
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

		// Act
		gotRead := IsTerminal(r)
		gotWrite := IsTerminal(w)

		// Assert
		if gotRead || gotWrite {
			t.Errorf("IsTerminal(読み口)=%v, IsTerminal(書き口)=%v, want どちらも false", gotRead, gotWrite)
		}
	})

	t.Run("ふつうのファイルは端末ではない", func(t *testing.T) {
		// Arrange
		f, err := os.Create(filepath.Join(t.TempDir(), "plain.txt"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })

		// Act
		got := IsTerminal(f)

		// Assert
		if got {
			t.Error("IsTerminal = true, want false")
		}
	})
}
