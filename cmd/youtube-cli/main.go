// youtube-cli は、YouTube へ限定公開で動画を上げるコマンドラインツール。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/wadoyoka/youtube-cli/internal/cli"
	"github.com/wadoyoka/youtube-cli/internal/repository/auth"
	"github.com/wadoyoka/youtube-cli/internal/repository/browser"
	"github.com/wadoyoka/youtube-cli/internal/repository/config"
	"github.com/wadoyoka/youtube-cli/internal/repository/gcloud"
	"github.com/wadoyoka/youtube-cli/internal/repository/terminal"
	"github.com/wadoyoka/youtube-cli/internal/repository/youtubeapi"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/account"
	"github.com/wadoyoka/youtube-cli/internal/usecase/setup"
)

func main() {
	// Ctrl-C だけでなく、エージェントの実行環境などが送る SIGTERM でも止める。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx)
	stop()
	os.Exit(code)
}

func run(ctx context.Context) int {
	onWindows := runtime.GOOS == "windows"
	dir, err := config.DefaultDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		return 1
	}
	configDir := config.NewDir(dir)
	policy := config.NewPolicyFile(dir)
	keys := auth.NewKeyStore(dir, os.Getenv("YOUTUBE_CLI_KEYRING"))
	authn := auth.NewAuth(dir, keys, config.NewLockFile(dir).Lock)
	remote := usecase.Remote{
		Open:   youtubeapi.NewOpener(authn),
		State:  config.NewStateFile(dir),
		Locker: config.NewLockFile(dir),
	}
	login := account.Login{Auth: authn, Remote: remote}
	status := account.Status{Dir: configDir, Auth: authn, Remote: remote}
	initUC := setup.Init{Dir: configDir, Policy: policy}
	return cli.Execute(ctx, cli.Deps{
		Init:   initUC,
		Login:  login,
		Logout: account.Logout{Auth: authn},
		Status: status,
		Policy: policy,
		Setup: setup.Setup{
			Init:    initUC,
			Policy:  policy,
			Auth:    authn,
			Cloud:   gcloud.NewGCloud(),
			Login:   login,
			Status:  status,
			OpenURL: browser.Open,
			Windows: onWindows,
		},
		WindowsPaths: onWindows,
		Interactive:  terminal.IsTerminal(os.Stdin) && terminal.IsTerminal(os.Stderr),
		Stdin:        os.Stdin,
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
	}, os.Args[1:])
}
