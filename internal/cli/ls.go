package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wadoyoka/youtube-cli/internal/usecase/video"
)

// lsItem は、ls --json の1要素。
type lsItem struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	URL           string    `json:"url"`
	PrivacyStatus string    `json:"privacyStatus"`
	UploadStatus  string    `json:"uploadStatus"`
	PublishedAt   time.Time `json:"publishedAt"`
	Tags          []string  `json:"tags"`
}

func newLsCommand(d Deps, asJSON *bool) *cobra.Command {
	long := new(bool)
	playlistRef := new(string)
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "このツールで上げた動画を新しい順に表示する",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			videos, err := d.List.Run(cmd.Context(), video.ListInput{PlaylistRef: *playlistRef})
			if err != nil {
				return err
			}
			items := make([]lsItem, 0, len(videos))
			for _, v := range videos {
				tags := v.Tags
				if tags == nil {
					tags = []string{}
				}
				items = append(items, lsItem{
					ID: v.ID, Title: v.Title, URL: v.URL(), PrivacyStatus: v.PrivacyStatus,
					UploadStatus: v.UploadStatus, PublishedAt: v.PublishedAt, Tags: tags,
				})
			}
			if *asJSON {
				return writeJSON(d.Stdout, items)
			}
			for _, it := range items {
				cols := []string{it.Title, it.URL, it.PrivacyStatus + "/" + it.UploadStatus}
				if *long {
					cols = append([]string{it.ID}, cols...)
					cols = append(cols, it.PublishedAt.Format(time.RFC3339), strings.Join(it.Tags, ","))
				}
				if _, err := fmt.Fprintln(d.Stdout, strings.Join(cols, "\t")); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(long, "long", "l", false, "ID・公開日時・タグも表示する")
	cmd.Flags().StringVar(playlistRef, "playlist", "", "このツールで作ったこの再生リスト（名前か ID）に入っている動画だけを表示する")
	return cmd
}
