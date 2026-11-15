// Package account は、ログイン・ログアウト・状態の確認（login・logout・status）の手順を持つ。
package account

import (
	"context"
	"fmt"
	"io"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// Login は、Google にログインして、YouTube のチャンネルを確かめる。
type Login struct {
	Auth   usecase.Authenticator
	Remote usecase.Remote
}

// LoginInput は、login の入力。
type LoginInput struct {
	ClientSecretPath string
	NoBrowser        bool
	// SwitchChannel は、保存してあるチャンネルと違うチャンネルが選ばれても、切り替えを認める。
	SwitchChannel bool
	Prompt        io.Writer
	Input         io.Reader
}

// LoginResult は、login の結果。
type LoginResult struct {
	ChannelID    string   `json:"channelId"`
	ChannelTitle string   `json:"channelTitle"`
	ChannelURL   string   `json:"channelUrl"`
	Warnings     []string `json:"warnings,omitempty"`
}

// Run は、client_secret の取り込み、ログイン、チャンネルの確認と保存を順に行う。
// 保存してあるチャンネルと違うチャンネルが選ばれたら、SwitchChannel でない限り、新しいトークンを元に戻して KindConflict で止める。
func (u Login) Run(ctx context.Context, in LoginInput) (LoginResult, error) {
	if in.ClientSecretPath != "" {
		if err := u.Auth.ImportClientSecret(in.ClientSecretPath); err != nil {
			return LoginResult{}, err
		}
	} else {
		insp, err := u.Auth.Inspect()
		if err != nil {
			return LoginResult{}, err
		}
		if !insp.ClientSecret {
			msg := insp.Problem
			if msg == "" {
				msg = "初回は --client-secret <JSON のパス> を指定してください"
			}
			return LoginResult{}, domain.Errorf(domain.KindSetup, "%s", msg)
		}
	}
	restore, err := u.Auth.SnapshotToken()
	if err != nil {
		return LoginResult{}, err
	}
	if err := u.Auth.Login(ctx, usecase.LoginOptions{NoBrowser: in.NoBrowser, Prompt: in.Prompt, Input: in.Input}); err != nil {
		return LoginResult{}, err
	}
	yt, err := u.Remote.Open(ctx)
	if err != nil {
		return LoginResult{}, err
	}
	ch, err := yt.MyChannel(ctx)
	if err != nil {
		return LoginResult{}, err
	}
	warnings, err := u.saveChannel(ctx, ch, in.SwitchChannel, restore)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{ChannelID: ch.ID, ChannelTitle: ch.Title, ChannelURL: ch.URL(), Warnings: warnings}, nil
}

// saveChannel は、ch を保存する。違うチャンネルが保存済みで switchChannel でなければ、restore してから KindConflict を返す。
func (u Login) saveChannel(ctx context.Context, ch domain.Channel, switchChannel bool, restore func() error) ([]string, error) {
	if u.Remote.Locker != nil {
		unlock, err := u.Remote.Locker.Lock(ctx)
		if err != nil {
			return nil, err
		}
		defer unlock()
	}
	saved, found, err := u.Remote.State.Channel()
	if err != nil {
		return nil, err
	}
	var warnings []string
	if found && saved.ID != ch.ID {
		if !switchChannel {
			err := domain.Errorf(domain.KindConflict, "選ばれたチャンネル（%s）が、保存してあるチャンネル（%s）と違います。切り替えるときは `youtube-cli login --switch-channel` を使ってください", ch.Title, saved.Title)
			if rerr := restore(); rerr != nil {
				return nil, err.Wrap(fmt.Errorf("元のログインに戻せませんでした: %w", rerr))
			}
			return nil, err
		}
		warnings = append(warnings, fmt.Sprintf("チャンネルを %s から %s に切り替えました", saved.Title, ch.Title))
	}
	if !found || saved != ch {
		if err := u.Remote.State.SetChannel(ch); err != nil {
			return nil, err
		}
	}
	return warnings, nil
}
