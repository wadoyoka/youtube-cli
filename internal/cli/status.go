package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStatusCommand(d Deps, asJSON *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "ログインの状態を表示する",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, runErr := d.Status.Run(cmd.Context())
			var werr error
			switch {
			case *asJSON:
				werr = writeJSON(d.Stdout, res)
			case res.LoggedIn:
				_, werr = fmt.Fprintf(d.Stdout, "設定ディレクトリ: %s\n鍵の置き場所: %s\nログイン: 済み\nチャンネル: %s（%s）\nURL: %s\n",
					res.ConfigDir, keyStoreLabel(res.KeyStore), res.ChannelTitle, res.ChannelID, res.ChannelURL)
			default:
				_, werr = fmt.Fprintf(d.Stdout, "設定ディレクトリ: %s\n鍵の置き場所: %s\nログイン: していません\n", res.ConfigDir, keyStoreLabel(res.KeyStore))
				if werr == nil && res.Problem != "" {
					_, werr = fmt.Fprintf(d.Stdout, "理由: %s\n", res.Problem)
				}
			}
			if runErr != nil {
				return runErr
			}
			return werr
		},
	}
}

// keyStoreLabel は、鍵の置き場所の種類を表示用の文言にする。
func keyStoreLabel(kind string) string {
	switch kind {
	case "os":
		return "OS のキーチェーン"
	case "file":
		return "設定ディレクトリのファイル（key）"
	default:
		return kind
	}
}
