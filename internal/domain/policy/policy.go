// Package policy は、実行を拒否するコマンドの一覧（policy.toml の中身）と、その判定を持つ。
package policy

import (
	"slices"
	"strings"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

// CommandNames は、拒否リストに書ける、実行できるコマンドの名前。
// playlist の子コマンドは playlist-<名前> で書く。
var CommandNames = []string{
	"setup", "login", "logout", "status", "upload", "thumbnail", "ls",
	"playlist-ls", "playlist-create", "playlist-add",
}

// DefaultTOML は、init が作る policy.toml の初期の内容。
// 再生リストの作成と追加は、既定で拒否しておく（設計書 6.5）。
const DefaultTOML = `# youtube-cli で実行を拒否するコマンドの一覧。
# ここに書いたコマンドを実行しようとすると、終了コード 7 で止まる。
# 書ける名前: setup, login, logout, status, upload, thumbnail, ls, playlist-ls, playlist-create, playlist-add
# upload --thumbnail は thumbnail も、upload --playlist は playlist-add も確かめる。
# 再生リストを使うときは、下の playlist-create と playlist-add を消す。
deny = ["playlist-create", "playlist-add"]
`

// Policy は、実行を拒否するコマンドの一覧。
type Policy struct{ Deny []string }

// Validate は、知らないコマンド名が含まれていれば KindDenied を返す。
func (p Policy) Validate() error {
	for _, name := range p.Deny {
		if !slices.Contains(CommandNames, name) {
			return domain.Errorf(domain.KindDenied, "policy.toml に知らないコマンド名があるため、すべてのコマンドを止めています: %s", name).
				WithDetails("書ける名前: " + strings.Join(CommandNames, ", "))
		}
	}
	return nil
}

// Check は、command が拒否されていれば KindDenied を返す。
func (p Policy) Check(command string) error {
	if slices.Contains(p.Deny, command) {
		return domain.Errorf(domain.KindDenied, "%s は policy.toml で拒否されています", command)
	}
	return nil
}
