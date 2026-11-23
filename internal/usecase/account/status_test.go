package account

import (
	"context"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
)

func newStatus(a *usecasetest.Auth, yt *usecasetest.YouTube, st *usecasetest.State) Status {
	return Status{
		Dir:  usecasetest.Dir{DirPath: "/cfg"},
		Auth: a,
		Remote: usecase.Remote{
			Open:   usecasetest.Opener(yt),
			State:  st,
			Locker: &usecasetest.Locker{},
		},
	}
}

func TestStatus(t *testing.T) {
	t.Run("ログイン済みならチャンネルの名前・ID・URL が入る", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{SecretSaved: true, TokenSaved: true, KeyStore: "os"}
		u := newStatus(a, &usecasetest.YouTube{Channel: chanA}, &usecasetest.State{Saved: chanA, Found: true})

		// Act
		got, err := u.Run(context.Background())

		// Assert
		want := StatusResult{
			ConfigDir: "/cfg", LoggedIn: true, KeyStore: "os",
			ChannelID: "UCaaa", ChannelTitle: "チャンネルA", ChannelURL: "https://www.youtube.com/channel/UCaaa",
		}
		if err != nil || got != want {
			t.Errorf("got %+v, %v; want %+v", got, err, want)
		}
	})

	t.Run("ログインしていなければ LoggedIn=false と KindSetup", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{KeyStore: "file"}
		u := newStatus(a, &usecasetest.YouTube{Channel: chanA}, &usecasetest.State{})

		// Act
		got, err := u.Run(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup || got.LoggedIn || got.ConfigDir != "/cfg" || got.KeyStore != "file" {
			t.Errorf("got %+v, %v", got, err)
		}
	})

	t.Run("読めない理由は Problem に入る", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{KeyStore: "os", Problem: "今の鍵で読めません"}
		u := newStatus(a, &usecasetest.YouTube{}, &usecasetest.State{})

		// Act
		got, err := u.Run(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup || got.Problem != "今の鍵で読めません" {
			t.Errorf("got %+v, %v", got, err)
		}
	})

	t.Run("トークンのチャンネルが保存したものと違えば KindSetup", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{TokenSaved: true, KeyStore: "os"}
		u := newStatus(a, &usecasetest.YouTube{Channel: chanB}, &usecasetest.State{Saved: chanA, Found: true})

		// Act
		got, err := u.Run(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup || got.LoggedIn {
			t.Errorf("got %+v, %v", got, err)
		}
	})
}
