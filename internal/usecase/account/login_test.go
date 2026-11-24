package account

import (
	"context"
	"errors"
	"slices"
	"strings"
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
	return newLoginWithLedger(a, yt, st, &usecasetest.Ledger{})
}

func newLoginWithLedger(a *usecasetest.Auth, yt *usecasetest.YouTube, st *usecasetest.State, l *usecasetest.Ledger) Login {
	return Login{
		Auth: a,
		Remote: usecase.Remote{
			Open:   usecasetest.Opener(yt),
			State:  st,
			Locker: &usecasetest.Locker{},
			Ledger: l,
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

	t.Run("違うチャンネルでも SwitchChannel なら保存し直し、ledger を空にする", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{SecretSaved: true}
		st := &usecasetest.State{Saved: chanA, Found: true}
		l := &usecasetest.Ledger{VideoList: []usecase.LedgerVideo{{ID: "v1"}}}
		u := newLoginWithLedger(a, &usecasetest.YouTube{Channel: chanB}, st, l)

		// Act
		_, err := u.Run(context.Background(), LoginInput{SwitchChannel: true})

		// Assert
		if err != nil || st.Saved != chanB || l.Clears != 1 || len(l.VideoList) != 0 {
			t.Errorf("err=%v saved=%+v clears=%d videos=%v", err, st.Saved, l.Clears, l.VideoList)
		}
	})

	t.Run("同じチャンネルや拒否した切り替えでは ledger を消さない", func(t *testing.T) {
		// Arrange
		l := &usecasetest.Ledger{}
		same := newLoginWithLedger(&usecasetest.Auth{SecretSaved: true}, &usecasetest.YouTube{Channel: chanA}, &usecasetest.State{Saved: chanA, Found: true}, l)
		refused := newLoginWithLedger(&usecasetest.Auth{SecretSaved: true}, &usecasetest.YouTube{Channel: chanB}, &usecasetest.State{Saved: chanA, Found: true}, l)

		// Act
		_, err1 := same.Run(context.Background(), LoginInput{SwitchChannel: true})
		_, err2 := refused.Run(context.Background(), LoginInput{})

		// Assert
		if err1 != nil || domain.KindOf(err2) != domain.KindConflict || l.Clears != 0 {
			t.Errorf("err1=%v err2=%v clears=%d", err1, err2, l.Clears)
		}
	})

	t.Run("Lock が失敗したら token を元に戻す", func(t *testing.T) {
		// Arrange
		boom := errors.New("lock")
		a := &usecasetest.Auth{SecretSaved: true}
		u := newLogin(a, &usecasetest.YouTube{Channel: chanA}, &usecasetest.State{})
		u.Remote.Locker = &usecasetest.Locker{LockErr: boom}

		// Act
		_, err := u.Run(context.Background(), LoginInput{})

		// Assert
		if !errors.Is(err, boom) || !slices.Contains(a.Calls, "restore") || a.TokenSaved {
			t.Errorf("err=%v calls=%v token=%v", err, a.Calls, a.TokenSaved)
		}
	})

	t.Run("保存したチャンネルを読めなければ token を元に戻す", func(t *testing.T) {
		// Arrange
		boom := errors.New("read")
		a := &usecasetest.Auth{SecretSaved: true}
		u := newLogin(a, &usecasetest.YouTube{Channel: chanA}, &usecasetest.State{ChannelErr: boom})

		// Act
		_, err := u.Run(context.Background(), LoginInput{})

		// Assert
		if !errors.Is(err, boom) || !slices.Contains(a.Calls, "restore") || a.TokenSaved {
			t.Errorf("err=%v calls=%v token=%v", err, a.Calls, a.TokenSaved)
		}
	})

	t.Run("チャンネルを保存できなければ token を元に戻す", func(t *testing.T) {
		// Arrange
		boom := errors.New("write")
		a := &usecasetest.Auth{SecretSaved: true}
		u := newLogin(a, &usecasetest.YouTube{Channel: chanA}, &usecasetest.State{SetErr: boom})

		// Act
		_, err := u.Run(context.Background(), LoginInput{})

		// Assert
		if !errors.Is(err, boom) || !slices.Contains(a.Calls, "restore") || a.TokenSaved {
			t.Errorf("err=%v calls=%v token=%v", err, a.Calls, a.TokenSaved)
		}
	})

	t.Run("切り替えで ledger を空にできなければ token を元に戻し、チャンネルは変えない", func(t *testing.T) {
		// Arrange
		boom := errors.New("clear")
		a := &usecasetest.Auth{SecretSaved: true}
		st := &usecasetest.State{Saved: chanA, Found: true}
		l := &usecasetest.Ledger{ClearErr: boom}
		u := newLoginWithLedger(a, &usecasetest.YouTube{Channel: chanB}, st, l)

		// Act
		_, err := u.Run(context.Background(), LoginInput{SwitchChannel: true})

		// Assert
		if !errors.Is(err, boom) || !slices.Contains(a.Calls, "restore") || a.TokenSaved {
			t.Errorf("err=%v calls=%v token=%v", err, a.Calls, a.TokenSaved)
		}
		if st.Saved != chanA || st.Sets != 0 {
			t.Errorf("チャンネルが変わった: %+v sets=%d", st.Saved, st.Sets)
		}
	})

	t.Run("MyChannel が失敗したら token を元に戻す", func(t *testing.T) {
		// Arrange
		boom := errors.New("quota")
		a := &usecasetest.Auth{SecretSaved: true}
		u := newLogin(a, &usecasetest.YouTube{ChannelErr: boom}, &usecasetest.State{})

		// Act
		_, err := u.Run(context.Background(), LoginInput{})

		// Assert
		if !errors.Is(err, boom) || !slices.Contains(a.Calls, "restore") || a.TokenSaved {
			t.Errorf("err=%v calls=%v token=%v", err, a.Calls, a.TokenSaved)
		}
	})

	t.Run("YouTube を開けなかったら token を元に戻す", func(t *testing.T) {
		// Arrange
		boom := errors.New("open")
		a := &usecasetest.Auth{SecretSaved: true}
		u := newLogin(a, &usecasetest.YouTube{}, &usecasetest.State{})
		u.Remote.Open = func(context.Context) (usecase.YouTube, error) { return nil, boom }

		// Act
		_, err := u.Run(context.Background(), LoginInput{})

		// Assert
		if !errors.Is(err, boom) || !slices.Contains(a.Calls, "restore") {
			t.Errorf("err=%v calls=%v", err, a.Calls)
		}
	})

	t.Run("元に戻すのにも失敗したら両方を伝える", func(t *testing.T) {
		// Arrange
		boom := errors.New("quota")
		rboom := errors.New("restore-failed")
		a := &usecasetest.Auth{SecretSaved: true, RestoreErr: rboom}
		u := newLogin(a, &usecasetest.YouTube{ChannelErr: boom}, &usecasetest.State{})

		// Act
		_, err := u.Run(context.Background(), LoginInput{})

		// Assert
		if err == nil || !errors.Is(err, boom) || !strings.Contains(err.Error(), "restore-failed") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("チャンネルがないアカウントは KindSetup で token を元に戻す", func(t *testing.T) {
		// Arrange
		a := &usecasetest.Auth{SecretSaved: true}
		noChannel := domain.Errorf(domain.KindSetup, "チャンネルがありません")
		u := newLogin(a, &usecasetest.YouTube{ChannelErr: noChannel}, &usecasetest.State{})

		// Act
		_, err := u.Run(context.Background(), LoginInput{})

		// Assert
		if domain.KindOf(err) != domain.KindSetup || !slices.Contains(a.Calls, "restore") || a.TokenSaved {
			t.Errorf("err=%v calls=%v token=%v", err, a.Calls, a.TokenSaved)
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
