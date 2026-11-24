package cli

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestProgressPrinter(t *testing.T) {
	t.Run("パスと割合を同じ行で書き換え、100% で改行する", func(t *testing.T) {
		// Arrange
		var buf bytes.Buffer
		p := newProgressPrinter(&buf)

		// Act
		p.Report("/a.mp4", 0, 200)
		p.Report("/a.mp4", 100, 200)
		p.Report("/a.mp4", 200, 200)
		p.Finish()

		// Assert
		if want := "\r\x1b[K/a.mp4  0%\r\x1b[K/a.mp4  50%\r\x1b[K/a.mp4  100%\n"; buf.String() != want {
			t.Errorf("got=%q want=%q", buf.String(), want)
		}
	})

	t.Run("割合が変わらなければ書き直さない", func(t *testing.T) {
		// Arrange
		var buf bytes.Buffer
		p := newProgressPrinter(&buf)

		// Act
		p.Report("/a.mp4", 0, 1000)
		p.Report("/a.mp4", 9, 1000)

		// Assert
		if want := "\r\x1b[K/a.mp4  0%"; buf.String() != want {
			t.Errorf("got=%q want=%q", buf.String(), want)
		}
	})

	t.Run("0 バイトのファイルは 100%", func(t *testing.T) {
		// Arrange
		var buf bytes.Buffer
		p := newProgressPrinter(&buf)

		// Act
		p.Report("/empty.txt", 0, 0)

		// Assert
		if want := "\r\x1b[K/empty.txt  100%\n"; buf.String() != want {
			t.Errorf("got=%q want=%q", buf.String(), want)
		}
	})

	t.Run("終わっていない行は Finish で消す", func(t *testing.T) {
		// Arrange
		var buf bytes.Buffer
		p := newProgressPrinter(&buf)
		p.Report("/a.mp4", 50, 100)

		// Act
		p.Finish()

		// Assert
		if want := "\r\x1b[K/a.mp4  50%\r\x1b[K"; buf.String() != want {
			t.Errorf("got=%q want=%q", buf.String(), want)
		}
	})

	t.Run("nil の Finish は何もしない", func(_ *testing.T) {
		// Arrange
		var p *progressPrinter

		// Act
		p.Finish()

		// Assert（落ちなければよい）
	})

	t.Run("同時に呼んでも行が混ざらない", func(t *testing.T) {
		// Arrange
		var buf bytes.Buffer
		p := newProgressPrinter(&buf)
		var wg sync.WaitGroup

		// Act
		for i := range 8 {
			wg.Go(func() {
				for done := int64(0); done <= 100; done++ {
					p.Report(fmt.Sprintf("/f%d", i), done, 100)
				}
			})
		}
		wg.Wait()
		p.Finish()

		// Assert
		segments := strings.Split(buf.String(), "\r\x1b[K")
		line := regexp.MustCompile(`^/f\d  \d{1,3}%\n?$`)
		for _, s := range segments[1:] {
			if s != "" && !line.MatchString(s) {
				t.Fatalf("混ざった行: %q", s)
			}
		}
		if got := strings.Count(buf.String(), "100%\n"); got != 8 {
			t.Errorf("100%% の行=%d want=8", got)
		}
	})
}
