package policy

import (
	"slices"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

func TestPolicy(t *testing.T) {
	t.Run("拒否リストにあるコマンドは拒否される", func(t *testing.T) {
		// Arrange
		p := Policy{Deny: []string{"upload"}}

		// Act
		err := p.Check("upload")

		// Assert
		if domain.KindOf(err) != domain.KindDenied || !strings.Contains(err.Error(), "upload") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("拒否リストにないコマンドは通る", func(t *testing.T) {
		// Arrange
		p := Policy{Deny: []string{"upload"}}

		// Act
		err := p.Check("ls")
		// Assert
		if err != nil {
			t.Errorf("got %v", err)
		}
	})

	t.Run("知らないコマンド名は検証で弾かれる", func(t *testing.T) {
		// Arrange
		p := Policy{Deny: []string{"uplaod"}}

		// Act
		err := p.Validate()

		// Assert
		if domain.KindOf(err) != domain.KindDenied || !strings.Contains(err.Error(), "uplaod") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("正しいコマンド名は検証を通る", func(t *testing.T) {
		// Arrange
		p := Policy{Deny: CommandNames}

		// Act
		err := p.Validate()
		// Assert
		if err != nil {
			t.Errorf("got %v", err)
		}
	})

	t.Run("既定の内容にすべてのコマンド名が書かれている", func(t *testing.T) {
		// Arrange / Act / Assert
		for _, n := range CommandNames {
			if !strings.Contains(DefaultTOML, n) {
				t.Errorf("%q がない", n)
			}
		}
	})

	t.Run("CommandNames に setup がある", func(t *testing.T) {
		// Arrange / Act
		ok := slices.Contains(CommandNames, "setup")

		// Assert
		if !ok {
			t.Error("setup がない")
		}
	})

	t.Run("ひな形の「書ける名前」に setup がある", func(t *testing.T) {
		// Arrange / Act
		ok := strings.Contains(DefaultTOML, "# 書ける名前: setup, login, logout, status, upload, thumbnail, ls, playlist-ls, playlist-create, playlist-add")

		// Assert
		if !ok {
			t.Errorf("ひな形: %s", DefaultTOML)
		}
	})

	t.Run("既定の内容では playlist-create と playlist-add を拒否する", func(t *testing.T) {
		// Arrange / Act
		ok := strings.Contains(DefaultTOML, "\ndeny = [\"playlist-create\", \"playlist-add\"]\n")

		// Assert
		if !ok {
			t.Errorf("ひな形: %s", DefaultTOML)
		}
	})

	t.Run("playlist-add は書ける名前", func(t *testing.T) {
		// Arrange
		p := Policy{Deny: []string{"playlist-add"}}

		// Act
		err := p.Validate()
		// Assert
		if err != nil {
			t.Errorf("got %v", err)
		}
	})

	t.Run("gdrive-cli にあった mkdir は知らない名前", func(t *testing.T) {
		// Arrange
		p := Policy{Deny: []string{"mkdir"}}

		// Act
		err := p.Validate()

		// Assert
		if domain.KindOf(err) != domain.KindDenied {
			t.Errorf("got %v", err)
		}
	})
}
