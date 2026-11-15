// youtube-cli は、YouTube へ限定公開で動画を上げるコマンドラインツール。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/wadoyoka/youtube-cli/internal/cli"
	"github.com/wadoyoka/youtube-cli/internal/repository/auth"
	"github.com/wadoyoka/youtube-cli/internal/repository/config"
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
	dir, err := config.DefaultDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		return 1
	}
	configDir := config.NewDir(dir)
	policy := config.NewPolicyFile(dir)
	keys := auth.NewKeyStore(dir, os.Getenv("YOUTUBE_CLI_KEYRING"))
	authn := auth.NewAuth(dir, keys, config.NewLockFile(dir).Lock)
	return cli.Execute(ctx, cli.Deps{
		Init:   setup.Init{Dir: configDir, Policy: policy},
		Logout: account.Logout{Auth: authn},
		Policy: policy,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}, os.Args[1:])
}
