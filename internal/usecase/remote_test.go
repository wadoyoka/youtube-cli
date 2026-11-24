package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
)

func TestRemoteConnect(t *testing.T) {
	mine := domain.Channel{ID: "UC1", Title: "自分", UploadsPlaylistID: "UU1"}

	t.Run("保存したチャンネルと同じなら Session を返す", func(t *testing.T) {
		// Arrange
		r := usecase.Remote{
			Open:  usecasetest.Opener(&usecasetest.YouTube{Channel: mine}),
			State: &usecasetest.State{Saved: mine, Found: true},
		}

		// Act
		s, err := r.Connect(context.Background())

		// Assert
		if err != nil || s.Channel != mine || s.YT == nil {
			t.Errorf("s=%+v err=%v", s, err)
		}
	})

	t.Run("保存したチャンネルと違えば KindSetup", func(t *testing.T) {
		// Arrange
		other := domain.Channel{ID: "UC2", Title: "別"}
		r := usecase.Remote{
			Open:  usecasetest.Opener(&usecasetest.YouTube{Channel: other}),
			State: &usecasetest.State{Saved: mine, Found: true},
		}

		// Act
		_, err := r.Connect(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("保存したチャンネルがなければ KindSetup", func(t *testing.T) {
		// Arrange
		r := usecase.Remote{
			Open:  usecasetest.Opener(&usecasetest.YouTube{Channel: mine}),
			State: &usecasetest.State{},
		}

		// Act
		_, err := r.Connect(context.Background())

		// Assert
		if domain.KindOf(err) != domain.KindSetup {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("MyChannel の失敗はそのまま返す", func(t *testing.T) {
		// Arrange
		boom := errors.New("boom")
		r := usecase.Remote{
			Open:  usecasetest.Opener(&usecasetest.YouTube{ChannelErr: boom}),
			State: &usecasetest.State{Saved: mine, Found: true},
		}

		// Act
		_, err := r.Connect(context.Background())

		// Assert
		if !errors.Is(err, boom) {
			t.Errorf("err = %v", err)
		}
	})
}
