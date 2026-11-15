package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	dv "github.com/wadoyoka/youtube-cli/internal/domain/video"
	"github.com/wadoyoka/youtube-cli/internal/usecase/video"
)

// uploadJSON は、upload --json の結果。
type uploadJSON struct {
	VideoID      string    `json:"videoId"`
	Title        string    `json:"title"`
	URL          string    `json:"url"`
	StudioURL    string    `json:"studioUrl"`
	Privacy      string    `json:"privacyStatus"`
	UploadStatus string    `json:"uploadStatus"`
	Existing     bool      `json:"existing"`
	Thumbnail    *stepJSON `json:"thumbnail"` // --thumbnail がなければ null
	Warnings     []string  `json:"warnings"`
}

// stepJSON は、上げたあとの手順（サムネイルの設定など）の結果。
type stepJSON struct {
	Done  bool   `json:"done"`
	Error string `json:"error,omitempty"`
}

// uploadFlags は、upload のフラグ。
type uploadFlags struct {
	title, description, descriptionFile, category, thumbnail string
	tags                                                     []string
	madeForKids, syntheticMedia, skipExisting                bool
}

func newUploadCommand(d Deps, asJSON *bool) *cobra.Command {
	var f uploadFlags
	cmd := &cobra.Command{
		Use:   "upload <動画>",
		Short: "動画を限定公開で上げる",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.thumbnail != "" {
				if err := checkPolicy(d, "thumbnail"); err != nil {
					return err
				}
			}
			meta, err := f.meta(cmd)
			if err != nil {
				return err
			}
			in := video.UploadInput{Path: args[0], Meta: meta, ThumbnailPath: f.thumbnail, SkipExisting: f.skipExisting}
			var progress *progressPrinter
			if d.Progress && !*asJSON {
				progress = newProgressPrinter(d.Stderr)
				total := fileSize(args[0])
				in.Progress = func(sent int64) { progress.Report(args[0], sent, total) }
			}
			res, err := d.Upload.Run(cmd.Context(), in)
			progress.Finish()
			if res.Video.ID == "" {
				return err
			}
			werr := writeUploadResult(d, *asJSON, res, f.thumbnail != "")
			if err != nil {
				return err
			}
			return werr
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&f.title, "title", "", "タイトル（省くと、拡張子を除いたファイル名）")
	fl.StringVar(&f.description, "description", "", "説明")
	fl.StringVar(&f.descriptionFile, "description-file", "", "説明を読むファイル（--description と同時には指定できない）")
	fl.StringArrayVar(&f.tags, "tag", nil, "タグ（繰り返して複数付けられる。目印の youtube-cli は自動で足す）")
	fl.StringVar(&f.category, "category", dv.DefaultCategory, "カテゴリの ID")
	fl.BoolVar(&f.madeForKids, "made-for-kids", false, "子ども向けとして申告する")
	fl.BoolVar(&f.syntheticMedia, "synthetic-media", false, "改変・合成されたリアルなコンテンツとして申告する")
	fl.StringVar(&f.thumbnail, "thumbnail", "", "上げたあとで設定するサムネイル（jpg か png、2 MiB まで）")
	fl.BoolVar(&f.skipExisting, "skip-existing", false, "同じタイトルの CLI 動画があれば、上げずにそれを使い、残りの手順だけを行う")
	return cmd
}

// meta は、フラグから動画の項目を作る。--description と --description-file の同時指定は KindUsage。
// 説明のファイルがなければ KindNotFound。
func (f uploadFlags) meta(cmd *cobra.Command) (dv.Meta, error) {
	m := dv.Meta{
		Title: f.title, Description: f.description, Tags: f.tags, CategoryID: f.category,
		MadeForKids: f.madeForKids, SyntheticMedia: f.syntheticMedia,
	}
	if !cmd.Flags().Changed("description-file") {
		return m, nil
	}
	if cmd.Flags().Changed("description") {
		return dv.Meta{}, domain.Errorf(domain.KindUsage, "--description と --description-file は同時には指定できません")
	}
	b, err := os.ReadFile(f.descriptionFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return dv.Meta{}, domain.Errorf(domain.KindNotFound, "説明のファイルが見つかりません: %s", f.descriptionFile).Wrap(err)
	case err != nil:
		return dv.Meta{}, domain.Errorf(domain.KindUsage, "説明のファイルを読めません: %s", f.descriptionFile).Wrap(err)
	}
	m.Description = string(b)
	return m, nil
}

// fileSize は、進み具合の割合を出すために、ファイルの大きさを返す。分からなければ 0。
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// writeUploadResult は、upload の結果を書く。thumbnail は --thumbnail を付けたかどうか。
// JSON でなければ、警告を標準エラーに、「<uploaded か existing>\t<タイトル>\t<URL>\t<Studio の URL>」を標準出力に書く。
// 失敗した手順は、エラーとして標準エラーに出るので、ここでは書かない。
func writeUploadResult(d Deps, asJSON bool, res video.UploadResult, thumbnail bool) error {
	if asJSON {
		out := uploadJSON{
			VideoID: res.Video.ID, Title: res.Video.Title, URL: res.URL, StudioURL: res.StudioURL,
			Privacy: res.Video.PrivacyStatus, UploadStatus: res.Video.UploadStatus, Existing: res.Existing,
			Warnings: res.Warnings,
		}
		if out.Warnings == nil {
			out.Warnings = []string{}
		}
		if thumbnail {
			out.Thumbnail = &stepJSON{Done: res.Thumbnail.Done, Error: res.Thumbnail.Error}
		}
		return writeJSON(d.Stdout, out)
	}
	for _, w := range res.Warnings {
		_, _ = fmt.Fprintf(d.Stderr, "注意: %s\n", w)
	}
	action := "uploaded"
	if res.Existing {
		action = "existing"
	}
	_, err := fmt.Fprintf(d.Stdout, "%s\t%s\t%s\t%s\n", action, res.Video.Title, res.URL, res.StudioURL)
	return err
}
