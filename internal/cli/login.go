package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wadoyoka/youtube-cli/internal/usecase/account"
)

func newLoginCommand(d Deps, asJSON *bool) *cobra.Command {
	var secretPath string
	var noBrowser, switchChannel bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Google にログインし、YouTube のチャンネルを確かめる",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := d.Login.Run(cmd.Context(), account.LoginInput{
				ClientSecretPath: secretPath,
				NoBrowser:        noBrowser,
				SwitchChannel:    switchChannel,
				Prompt:           d.Stderr,
				Input:            d.Stdin,
			})
			if err != nil {
				return err
			}
			if *asJSON {
				return writeJSON(d.Stdout, res)
			}
			for _, w := range res.Warnings {
				_, _ = fmt.Fprintf(d.Stderr, "注意: %s\n", w)
			}
			_, err = fmt.Fprintf(d.Stdout, "ログインしました: %s\nチャンネル: %s\n", res.ChannelTitle, res.ChannelURL)
			return err
		},
	}
	cmd.Flags().StringVar(&secretPath, "client-secret", "", "OAuth クライアントの JSON のパス（初回のみ）")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "ブラウザを開かず、URL を表示して認可コードを貼り付ける")
	cmd.Flags().BoolVar(&switchChannel, "switch-channel", false, "保存してあるチャンネルと違うチャンネルでのログインを認める")
	return cmd
}
