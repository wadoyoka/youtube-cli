package video

import (
	"context"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// List は、このツールで上げた動画を一覧にする。
type List struct {
	Remote usecase.Remote
}

// ListInput は、List の入力。
type ListInput struct{}

// Run は、このツールで上げた動画を、新しい順に返す。
func (u List) Run(ctx context.Context, _ ListInput) ([]domain.Video, error) {
	s, err := u.Remote.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return Catalog(u).Videos(ctx, s)
}
