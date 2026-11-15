# 開発

```bash
mise install        # ツールを入れる。lefthook のフックも入る
make build          # リポジトリの直下に ./youtube-cli を作る。macOS では hardened runtime を付ける
make install        # ~/.local/bin に入れる（BINDIR で変えられる）。macOS では hardened runtime を付ける
make lint           # golangci-lint run ./...
make fmt            # golangci-lint fmt
make test           # go test ./...
```

- macOS では、`make build` で作る（`go build` のままでは hardened runtime が付かず、キーチェーンを使えない）。ソースを変えてビルドし直すと、最初の実行でダイアログが出る（同じソースから作り直しただけなら、中身が同じなので出ない）。`make install` は、`~/.local/bin` に入れるときに使う。
- Windows では、`make build` と `make install` は `go build` だけで動く（hardened runtime は macOS だけ）。`make install` は `%USERPROFILE%\.local\bin` に入れる。
- macOS から Windows 向けのコンパイルを確かめるには、`GOOS=windows go build ./...` と `GOOS=windows go vet ./...` を実行する。Windows の上でのテストは、GitHub の Actions の画面で ci を「Run workflow」で起動する（または `gh workflow run ci.yml --ref <ブランチ>`）。手で起動したときだけ動く。
- ツールの版は `.mise.toml` に固定してある（go、golangci-lint、trivy、lefthook、node、pnpm）。CI も同じ版を使う。
- コミット時は lefthook が、gofmt、golangci-lint、go test、trivy（secret と vuln）、commitlint を走らせる。
- コミットメッセージの形式は `type: :emoji: 件名`（件名は日本語）。type と絵文字の対応は `commitlint.config.mjs` に従う。`--no-verify` は使わない。
- 層は cli、usecase、domain、repository の4つ。依存の向きは `internal/archtest` と golangci-lint の depguard が確かめる。削除・更新・公開範囲の変更の API を本体のコードから呼べないことと、`privacyStatus` に限定公開以外を入れないことも、archtest が機械的に確かめている。層ごとの置き場所は [CLAUDE.md](../CLAUDE.md)。
- タスクの管理には bd（beads）をステルスモードで使う（`bd ready`、`bd create`、`bd update <id> --status in_progress`、`bd close <id>`）。`.beads/` は git に入らない。
- 試し用のファイルは `rm` ではなく、`trash/` へ `mv` で片付ける。
- CI は commitlint、golangci-lint、`go test`、trivy を走らせる。毎日の trivy、Renovate による依存の更新、semantic-release によるリリースは [ci.md](ci.md) にある。
- main に入ると、コミットの type で次の版が決まる（feat はマイナー、fix・perf・revert はパッチ、ほかはリリースしない）。
- AI エージェント向けのスキルは `agents/` にある（[agents/README.md](../agents/README.md)）。

## E2E テスト

本物の YouTube を使う確かめは、CLI の E2E スクリプト（`tests/scripts/e2e-cli.sh`）だけ。Go の E2E テストはない。

**このスクリプトは、クォータを使い、CLI で消せない本物の限定公開の動画（と、再生リスト）を残す。** 人が必要なときだけ動かす。Claude などの AI は動かさない。

```bash
tests/scripts/e2e-cli.sh                                    # 確認の入力がなければ止まる
YOUTUBE_CLI_E2E=yes tests/scripts/e2e-cli.sh                # PATH の youtube-cli（make install で入れたもの）
YOUTUBE_CLI_E2E=yes YOUTUBE_CLI_BIN=./youtube-cli tests/scripts/e2e-cli.sh   # make build で作ったもの
YOUTUBE_CLI_E2E=yes YOUTUBE_CLI_E2E_PLAYLISTS=yes tests/scripts/e2e-cli.sh   # 再生リストの確かめも行う
YOUTUBE_CLI_E2E=yes YOUTUBE_CLI_E2E_THUMBNAIL=yes tests/scripts/e2e-cli.sh   # サムネイルの確かめも行う（電話番号の確認が要る）
```

- 普段使っている youtube-cli（ログイン済み）で、そのまま動かす。スクリプトはログインしない。
- 動画は、タイトルが `e2e-cli-<日時>-<PID>` の数秒の動画を 1 本だけ上げる（ffmpeg で作る。1 日 100 本の上限のうちの 1 本を使う）。16 MiB 以上の分けて送る経路は確かめない。**youtube-cli には削除がないので、終わったら、最後に表示される動画を Studio で消す。** 失敗や中断のときも表示される。
- `YOUTUBE_CLI_E2E_PLAYLISTS=yes` のときだけ、再生リストを 6 つ作る。**これも youtube-cli では消せない。Studio で消す。** 拒否リストで `playlist-create`・`playlist-add` が止まっているときは、先に人が外す（[policy.md](policy.md)）。
- 手元には一時ディレクトリを作り、終わったら（失敗や中断のときも）消す。
- 設定ディレクトリには、最後の確かめのあいだだけ `ledger.json`（この PC の手元の記録）を脇へ移し、終わったら戻す。それ以外（`policy.toml`・状態）には触れない。`YOUTUBE_CLI_CONFIG_DIR` と `YOUTUBE_CLI_KEYRING` は、そのまま youtube-cli に渡る。
- 1 件ごとに ✓ か ✗ を出し、最後に件数を出す。✗ には、直前のコマンド、終了コード、標準出力と標準エラーの頭が付く。
- 終了コードは、0 がすべて通った、1 が通らなかったものがある、2 が前提が足りない（確認の入力がない、youtube-cli・jq・ffmpeg がない、`status` が通らない）、130 が中断。
- 本物でしか分からない 3 つを確かめる。(a) 上げた動画が `privacyStatus: unlisted` で返る。(b) 上げた直後の動画が `ls` に出る（手元の記録のため）。手元の記録を脇へ移しても、しばらく待てば YouTube の側から返って出る（最大 10 分待つ）。(c) 再生リストの確かめのとき、このツールで作った再生リストが 6 つ以上あっても、`playlist ls` と名前での指定がすべて見つける（ID で引くとき `maxResults` を送らなくても、全部返ることの確認）。
- 確かめないもの: `init`・`setup`・`login`・`logout`、拒否リスト（`policy.toml` を書き換えることになるため）。
- 要るものは bash（3.2 以上）、jq、ffmpeg（試験用の動画を作るため）。
