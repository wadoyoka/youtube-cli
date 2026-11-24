package account

import (
	"context"
	"slices"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
)

func TestLogout(t *testing.T) {
	t.Run("ログアウトするとトークンが消える", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{SecretSaved: true, TokenSaved: true}
		u := Logout{Auth: a}

		// Act
		err := u.Run(context.Background())
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if a.TokenSaved || !slices.Equal(a.Calls, []string{"logout"}) {
			t.Errorf("TokenSaved=%v Calls=%v", a.TokenSaved, a.Calls)
		}
	})
}
