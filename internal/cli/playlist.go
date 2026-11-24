package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wadoyoka/youtube-cli/internal/usecase/playlist"
)

// playlistItem は、playlist ls --json の1要素。
type playlistItem struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	URL           string `json:"url"`
	PrivacyStatus string `json:"privacyStatus"`
	ItemCount     int64  `json:"itemCount"`
}

// playlistCreateJSON は、playlist create --json の結果。
type playlistCreateJSON struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	PrivacyStatus string   `json:"privacyStatus"`
	Warnings      []string `json:"warnings"`
}

// playlistAddJSON は、playlist add --json の結果。
type playlistAddJSON struct {
	VideoID       string `json:"videoId"`
	PlaylistID    string `json:"playlistId"`
	PlaylistTitle string `json:"playlistTitle"`
	PlaylistURL   string `json:"playlistUrl"`
	Added         bool   `json:"added"` // false なら、すでに入っていた
}

// newPlaylistCommand は、playlist と子コマンド（ls・create・add）を作る。
// 拒否リストでは、子コマンドを playlist-<名前> で確かめる（root.go の policyName）。
func newPlaylistCommand(d Deps, asJSON *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "playlist",
		Short: "このツールで作る限定公開の再生リストを扱う",
		Args:  usageArgs(cobra.NoArgs),
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newPlaylistLsCommand(d, asJSON), newPlaylistCreateCommand(d, asJSON), newPlaylistAddCommand(d, asJSON))
	return cmd
}

func newPlaylistLsCommand(d Deps, asJSON *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "このツールで作った再生リストを表示する",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			pls, err := d.PlaylistList.Run(cmd.Context())
			if err != nil {
				return err
			}
			items := make([]playlistItem, 0, len(pls))
			for _, p := range pls {
				items = append(items, playlistItem{ID: p.ID, Title: p.Title, URL: p.URL(), PrivacyStatus: p.PrivacyStatus, ItemCount: p.ItemCount})
			}
			if *asJSON {
				return writeJSON(d.Stdout, items)
			}
			for _, it := range items {
				if _, err := fmt.Fprintf(d.Stdout, "%s\t%s\t%s\t%d\n", it.Title, it.URL, it.PrivacyStatus, it.ItemCount); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newPlaylistCreateCommand(d Deps, asJSON *bool) *cobra.Command {
	description := new(string)
	cmd := &cobra.Command{
		Use:   "create <名前>",
		Short: "限定公開の再生リストを作る（説明の最後に目印の行 [youtube-cli] を足す）",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := d.PlaylistCreate.Run(cmd.Context(), playlist.CreateInput{Title: args[0], Description: *description})
			if err != nil {
				return err
			}
			p := res.Playlist
			if *asJSON {
				out := playlistCreateJSON{ID: p.ID, Title: p.Title, URL: p.URL(), PrivacyStatus: p.PrivacyStatus, Warnings: res.Warnings}
				if out.Warnings == nil {
					out.Warnings = []string{}
				}
				return writeJSON(d.Stdout, out)
			}
			for _, w := range res.Warnings {
				_, _ = fmt.Fprintf(d.Stderr, "注意: %s\n", w)
			}
			_, err = fmt.Fprintf(d.Stdout, "created\t%s\t%s\n", p.Title, p.URL())
			return err
		},
	}
	cmd.Flags().StringVar(description, "description", "", "説明")
	return cmd
}

func newPlaylistAddCommand(d Deps, asJSON *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "add <動画 ID> <再生リストの名前か ID>",
		Short: "このツールで上げた動画を、このツールで作った限定公開の再生リストに入れる",
		Args:  usageArgs(cobra.ExactArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := d.PlaylistAdd.Run(cmd.Context(), playlist.AddInput{VideoID: args[0], PlaylistRef: args[1]})
			if err != nil {
				return err
			}
			p := res.Playlist
			if *asJSON {
				return writeJSON(d.Stdout, playlistAddJSON{VideoID: res.VideoID, PlaylistID: p.ID, PlaylistTitle: p.Title, PlaylistURL: p.URL(), Added: res.Added})
			}
			action := "added"
			if !res.Added {
				action = "exists"
			}
			_, err = fmt.Fprintf(d.Stdout, "%s\t%s\t%s\t%s\n", action, res.VideoID, p.Title, p.URL())
			return err
		},
	}
}
