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
	"github.com/wadoyoka/youtube-cli/internal/usecase/playlist"
	"github.com/wadoyoka/youtube-cli/internal/usecase/setup"
	"github.com/wadoyoka/youtube-cli/internal/usecase/video"
)

// Deps は、コマンドが使うものをまとめたもの。
type Deps struct {
	Init      setup.Init
	Login     account.Login
	Logout    account.Logout
	Status    account.Status
	Setup     setup.Setup
	List      video.List
	Upload    video.Upload
	Thumbnail video.Thumbnail
	// PlaylistList・PlaylistCreate・PlaylistAdd は、playlist ls・create・add。
	PlaylistList   playlist.List
	PlaylistCreate playlist.Create
	PlaylistAdd    playlist.Add
	Policy         usecase.PolicyStore
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
	// Interactive は、標準入力と標準エラーが両方とも端末なら true。
	Interactive bool
	// WindowsPaths は、Windows で動いているとき true。
	WindowsPaths bool
	// Progress は、送信の進み具合を標準エラーに出すとき true（標準エラーが端末で、ANSI の制御文字を使えるとき）。
	Progress bool
}

// 拒否リストを確かめないコマンド。playlist は子コマンドの入れ物で、それだけなら使い方を出す。
var unchecked = []string{"init", "setup", "help", "completion", "youtube-cli", "playlist"}

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
			name := policyName(cmd)
			if slices.Contains(unchecked, name) {
				return nil
			}
			return checkPolicy(d, name)
		},
	}
	root.SetIn(d.Stdin)
	root.SetOut(d.Stdout)
	root.SetErr(d.Stderr)
	root.PersistentFlags().BoolVar(asJSON, "json", false, "結果を JSON で出す")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError(err) })
	root.AddCommand(newInitCommand(d, asJSON), newLoginCommand(d, asJSON), newLogoutCommand(d, asJSON),
		newStatusCommand(d, asJSON), newSetupCommand(d, asJSON), newLsCommand(d, asJSON), newUploadCommand(d, asJSON), newThumbnailCommand(d, asJSON),
		newPlaylistCommand(d, asJSON))
	return root
}

// policyName は、拒否リストで cmd を確かめるときの名前を返す。playlist の子コマンドは playlist-<名前>（playlist ls なら playlist-ls）。
func policyName(cmd *cobra.Command) string {
	if p := cmd.Parent(); p != nil && p.Name() == "playlist" {
		return "playlist-" + cmd.Name()
	}
	return cmd.Name()
}

// checkPolicy は、policy.toml を読み、names のどれかが拒否されていれば KindDenied を返す。
// コマンドそのものはルートが確かめる。フラグで別の操作もするとき（upload --thumbnail など）は、その名前もここで確かめる。
func checkPolicy(d Deps, names ...string) error {
	p, err := d.Policy.Load()
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := p.Check(name); err != nil {
			return err
		}
	}
	return nil
}
