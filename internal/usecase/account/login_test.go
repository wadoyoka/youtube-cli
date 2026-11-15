package account

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
)

var (
	chanA = domain.Channel{ID: "UCaaa", Title: "チャンネルA", UploadsPlaylistID: "UUaaa"}
	chanB = domain.Channel{ID: "UCbbb", Title: "チャンネルB", UploadsPlaylistID: "UUbbb"}
)

func newLogin(a *usecasetest.Auth, yt *usecasetest.YouTube, st *usecasetest.State) Login {
	return Login{
		Auth: a,
		Remote: usecase.Remote{
			Open:   usecasetest.Opener(yt),
			State:  st,
			Locker: &usecasetest.Locker{},
		},
	}
}

func TestLogin(t *testing.T) {
	t.Run("保存したチャンネルがなければ保存する", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{SecretSaved: true}
		st := &usecasetest.State{}
		u := newLogin(a, &usecasetest.YouTube{Channel: chanA}, st)

		// Act
		got, err := u.Run(context.Background(), LoginInput{NoBrowser: true})
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		want := LoginResult{ChannelID: "UCaaa", ChannelTitle: "チャンネルA", ChannelURL: "https://www.youtube.com/channel/UCaaa"}
		if got.ChannelID != want.ChannelID || got.ChannelTitle != want.ChannelTitle || got.ChannelURL != want.ChannelURL {
			t.Errorf("got = %+v", got)
		}
		if st.Saved != chanA || st.Sets != 1 || !a.TokenSaved {
			t.Errorf("saved=%+v sets=%d token=%v", st.Saved, st.Sets, a.TokenSaved)
		}
	})

	t.Run("同じチャンネルなら成功し、restore は呼ばない", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{SecretSaved: true}
		st := &usecasetest.State{Saved: chanA, Found: true}
		u := newLogin(a, &usecasetest.YouTube{Channel: chanA}, st)

		// Act
		_, err := u.Run(context.Background(), LoginInput{})

		// Assert
		if err != nil || slices.Contains(a.Calls, "restore") || !a.TokenSaved {
			t.Errorf("err=%v calls=%v", err, a.Calls)
		}
	})

	t.Run("違うチャンネルなら restore を呼び KindConflict で止める", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{SecretSaved: true}
		st := &usecasetest.State{Saved: chanA, Found: true}
		u := newLogin(a, &usecasetest.YouTube{Channel: chanB}, st)

		// Act
		_, err := u.Run(context.Background(), LoginInput{})

		// Assert
		if domain.KindOf(err) != domain.KindConflict {
			t.Fatalf("err = %v", err)
		}
		if !slices.Contains(a.Calls, "restore") || a.TokenSaved {
			t.Errorf("calls=%v token=%v", a.Calls, a.TokenSaved)
		}
		if st.Saved != chanA || st.Sets != 0 {
			t.Errorf("state が変わった: %+v sets=%d", st.Saved, st.Sets)
		}
	})

	t.Run("違うチャンネルでも SwitchChannel なら保存し直す", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{SecretSaved: true}
		st := &usecasetest.State{Saved: chanA, Found: true}
		u := newLogin(a, &usecasetest.YouTube{Channel: chanB}, st)

		// Act
		got, err := u.Run(context.Background(), LoginInput{SwitchChannel: true})
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if st.Saved != chanB || got.ChannelID != "UCbbb" || !a.TokenSaved || slices.Contains(a.Calls, "restore") {
			t.Errorf("saved=%+v got=%+v calls=%v", st.Saved, got, a.Calls)
		}
		if len(got.Warnings) == 0 {
			t.Error("切り替えた警告が出ていない")
		}
	})

	t.Run("チャンネルがないアカウントは KindSetup", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{SecretSaved: true}
		noChannel := domain.Errorf(domain.KindSetup, "チャンネルがありません")
		u := newLogin(a, &usecasetest.YouTube{ChannelErr: noChannel}, &usecasetest.State{})

		// Act
		_, err := u.Run(context.Background(), LoginInput{})

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("--client-secret があれば取り込んでからログインする", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{}
		u := newLogin(a, &usecasetest.YouTube{Channel: chanA}, &usecasetest.State{})

		// Act
		_, err := u.Run(context.Background(), LoginInput{ClientSecretPath: "/x.json"})

		// Assert
		if err != nil || a.Imported != "/x.json" {
			t.Fatalf("err=%v imported=%q", err, a.Imported)
		}
		if i, l := slices.Index(a.Calls, "import"), slices.Index(a.Calls, "login"); i < 0 || l < i {
			t.Errorf("calls = %v", a.Calls)
		}
	})

	t.Run("client_secret がなく --client-secret もなければ KindSetup", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{}
		u := newLogin(a, &usecasetest.YouTube{Channel: chanA}, &usecasetest.State{})

		// Act
		_, err := u.Run(context.Background(), LoginInput{})

		// Assert
		if domain.KindOf(err) != domain.KindSetup || slices.Contains(a.Calls, "login") {
			t.Errorf("err=%v calls=%v", err, a.Calls)
		}
	})

	t.Run("スナップショットに失敗したらログインしない", func(t *testing.T) {
		// Arrange
		boom := errors.New("boom")
		a := &usecasetest.Auth{SecretSaved: true, SnapshotErr: boom}
		u := newLogin(a, &usecasetest.YouTube{Channel: chanA}, &usecasetest.State{})

		// Act
		_, err := u.Run(context.Background(), LoginInput{})

		// Assert
		if !errors.Is(err, boom) || slices.Contains(a.Calls, "login") {
			t.Errorf("err=%v calls=%v", err, a.Calls)
		}
	})
}
