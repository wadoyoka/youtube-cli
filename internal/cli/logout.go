package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLogoutCommand(d Deps, asJSON *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "保存したトークンを消す",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := d.Logout.Run(cmd.Context()); err != nil {
				return err
			}
			if *asJSON {
				return writeJSON(d.Stdout, map[string]bool{"loggedOut": true})
			}
			_, err := fmt.Fprintln(d.Stdout, "ログアウトしました")
			return err
		},
	}
}
