// Package cli は、コマンドラインの入口（引数の解釈と表示）を持つ。
package cli

import (
	"context"
	"io"
	"slices"

	"github.com/spf13/cobra"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/account"
	"github.com/wadoyoka/youtube-cli/internal/usecase/setup"
)

// Deps は、コマンドが使うものをまとめたもの。
type Deps struct {
	Init   setup.Init
	Login  account.Login
	Logout account.Logout
	Status account.Status
	Setup  setup.Setup
	Policy usecase.PolicyStore
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Interactive は、標準入力と標準エラーが両方とも端末なら true。
	Interactive bool
	// WindowsPaths は、Windows で動いているとき true。
	WindowsPaths bool
	// 後のタスクで、各 usecase のフィールドを足していく
}

// 拒否リストを確かめないコマンド。
var unchecked = []string{"init", "setup", "help", "completion", "youtube-cli"}

// Execute は、args を実行して終了コードを返す。
func Execute(ctx context.Context, d Deps, args []string) int {
	root := newRootCommand(d)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		asJSON, _ := root.PersistentFlags().GetBool("json")
		writeError(d.Stderr, asJSON, err)
		return domain.KindOf(err).ExitCode()
	}
	return 0
}

func usageError(err error) error {
	if err == nil {
		return nil
	}
	return domain.Errorf(domain.KindUsage, "%s", err.Error()).Wrap(err)
}

func usageArgs(f cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error { return usageError(f(cmd, args)) }
}

func newRootCommand(d Deps) *cobra.Command {
	asJSON := new(bool)
	root := &cobra.Command{
		Use:           "youtube-cli",
		Short:         "YouTube へ限定公開で動画を上げる CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          usageArgs(cobra.NoArgs),
		RunE:          func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if slices.Contains(unchecked, cmd.Name()) {
				return nil
			}
			p, err := d.Policy.Load()
			if err != nil {
				return err
			}
			return p.Check(cmd.Name())
		},
	}
	root.SetIn(d.Stdin)
	root.SetOut(d.Stdout)
	root.SetErr(d.Stderr)
	root.PersistentFlags().BoolVar(asJSON, "json", false, "結果を JSON で出す")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError(err) })
	root.AddCommand(newInitCommand(d, asJSON), newLoginCommand(d, asJSON), newLogoutCommand(d, asJSON),
		newStatusCommand(d, asJSON), newSetupCommand(d, asJSON))
	return root
}
