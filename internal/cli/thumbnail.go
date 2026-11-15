package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wadoyoka/youtube-cli/internal/usecase/video"
)

// thumbnailJSON は、thumbnail --json の結果。
type thumbnailJSON struct {
	VideoID string `json:"videoId"`
	URL     string `json:"url"`
}

func newThumbnailCommand(d Deps, asJSON *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "thumbnail <動画 ID> <画像>",
		Short: "このツールで上げた動画のサムネイルを設定する（jpg か png、2 MiB まで）",
		Args:  usageArgs(cobra.ExactArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := d.Thumbnail.Run(cmd.Context(), video.ThumbnailInput{VideoID: args[0], Path: args[1]})
			if err != nil {
				return err
			}
			if *asJSON {
				return writeJSON(d.Stdout, thumbnailJSON{VideoID: res.VideoID, URL: res.URL})
			}
			_, err = fmt.Fprintf(d.Stdout, "thumbnail\t%s\t%s\n", res.VideoID, res.URL)
			return err
		},
	}
}
