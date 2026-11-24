package account

import (
	"context"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// Status は、ログインの状態を調べる。
type Status struct {
	Dir    usecase.ConfigDir
	Auth   usecase.Authenticator
	Remote usecase.Remote
}

// StatusResult は、status の結果。
type StatusResult struct {
	ConfigDir    string `json:"configDir"`
	LoggedIn     bool   `json:"loggedIn"`
	KeyStore     string `json:"keyStore"`
	Problem      string `json:"problem,omitempty"`
	ChannelID    string `json:"channelId,omitempty"`
	ChannelTitle string `json:"channelTitle,omitempty"`
	ChannelURL   string `json:"channelUrl,omitempty"`
}

// Run は、状態を返す。ログインしていなければ、LoggedIn=false の結果と KindSetup を両方返す。
func (u Status) Run(ctx context.Context) (StatusResult, error) {
	res := StatusResult{ConfigDir: u.Dir.Path()}
	insp, err := u.Auth.Inspect()
	res.KeyStore = insp.KeyStore
	if err != nil {
		return res, err
	}
	if !insp.Token {
		res.Problem = insp.Problem
		msg := insp.Problem
		if msg == "" {
			msg = "ログインしていません。youtube-cli login を実行してください"
		}
		return res, domain.Errorf(domain.KindSetup, "%s", msg)
	}
	s, err := u.Remote.Connect(ctx)
	if err != nil {
		return res, err
	}
	res.LoggedIn = true
	res.ChannelID = s.Channel.ID
	res.ChannelTitle = s.Channel.Title
	res.ChannelURL = s.Channel.URL()
	return res, nil
}
