package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func created(b bool) string {
	if b {
		return "作成しました"
	}
	return "既にあります"
}

func newInitCommand(d Deps, asJSON *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "設定ディレクトリと拒否リストを用意する",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(*cobra.Command, []string) error {
			res, err := d.Init.Run()
			if err != nil {
				return err
			}
			if *asJSON {
				return writeJSON(d.Stdout, res)
			}
			_, err = fmt.Fprintf(d.Stdout, "設定ディレクトリ: %s（%s）\n拒否リスト: %s（%s）\n",
				res.ConfigDir, created(res.DirCreated), res.PolicyPath, created(res.PolicyCreated))
			return err
		},
	}
}
