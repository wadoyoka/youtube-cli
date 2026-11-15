# CLAUDE.md

YouTube へ限定公開だけで動画を上げる Go 製の CLI。gdrive-cli（`github.com/wadoyoka/gdrive-cli`）の共通部分をコピーして作っている。設計は `docs/superpowers/specs/`、実装計画は `docs/superpowers/plans/` に置く（手元だけ。`.gitignore` で除外し、コミットしない）。

## GitHub

- リポジトリは private で作る予定。**GitHub のリポジトリの作成と push は、利用者が許可するまで行わない。** 手元の git だけで作業する。

## 文書の置き場所

- `README.md` は入門だけ（概要、gcloud と youtube-cli のインストール、`setup`、アップロード）。コマンドの細かい動きや注意は `docs/` の各ファイルに書き、README からは表でリンクする。
- AI エージェント向けのスキルは `agents/skills/<name>/` に置き、`agents/setup.sh` で `~/.claude/skills/` などへリンクして配る。CLI の振る舞い（フラグ、JSON の形、終了コード）を変えたら、`docs/commands.md` と `agents/skills/youtube-cli/`（`SKILL.md`・`references/commands.md`）もそろえる。
- `agents/setup.sh` を試すときは、`HOME` を scratchpad などの仮のディレクトリに向ける（本物の `~/.claude/` にリンクを張らない）。

## 試し用のファイルの片付け

試し用に作ったファイルやディレクトリを片付けるときは、`rm` を使わず repo 直下の `trash/` へ `mv` する。`rm` は取り消せず、実行の前に確認を求められることも多いため。

- 例: `mv probe-dir trash/`
- `trash/` の中身は git に入らない。`trash/.gitignore` が `.gitignore`・`.gitkeep`・`go.mod` 以外を除外する。golangci-lint や trivy の対象にもならない。
- `trash/go.mod` は git に入っている。`trash/` を別の Go モジュールにして、`./...` や golangci-lint が `trash/` の中の Go のファイルを見ないようにするためのもの。消さない。
- 同じ名前のファイルは上書きしてよい。同じ名前のディレクトリがあると `mv` が失敗するので、名前を変えて入れる。
- `trash/` がないブランチや worktree では、`mkdir -p trash` してから入れる。
- `trash/` を空にするのは人がやる。

## コミットメッセージ

- 形式は `type: :emoji: 件名`（件名は日本語）。type と絵文字の対応は `commitlint.config.mjs` に従う。
- Claude のセッションの URL（`Claude-Session:` の行など）は入れない（commitlint の `no-claude-session-url` が落とす）。
- `--no-verify` は使わない。
- main に入ると semantic-release がコミットの type で次の版を決める（feat はマイナー、fix・perf・revert はパッチ、ほかはリリースしない。`docs/ci.md`）。振る舞いの変わらない変更に feat や fix を使わない。

## YouTube

- Claude は、本物の YouTube に動画を上げるコマンドや E2E スクリプトを実行しない（クォータを使い、CLI で消せない動画が残るため）。
- 公開範囲は限定公開（`unlisted`）だけ。`privacyStatus` には `video.PrivacyUnlisted` 以外を入れない（archtest が確かめる）。
- 削除・更新・公開範囲の変更の API は呼ばない。

## gcloud

- Claude は、本物の gcloud で GCP を変えるコマンド（`projects create`、`services enable` など）を実行しない。`youtube-cli setup` を本物の gcloud で動かすのは利用者。
- テストは偽物の gcloud と偽物のキーチェーンで行う。本物の OS のキーチェーンに触れるテストを書かない。
- 実装がまだないテストを、失敗するのを見るために動かすときも同じ。古い配線のままだと、`NewKeyStore(dir, "os")` が本物のキーチェーンに届くことがある。プラットフォームごとの鍵置き場のテストは、先に具体的な型を確かめる。

## キーチェーン

- Claude は、`make build` で作った `./youtube-cli` を `--help` 以外で実行しない（hardened runtime が付いていて、本物のキーチェーンに触れるため）。`make install` で入れたものも同じ。
- macOS のキーチェーンの振る舞いを確かめるのは利用者。
- `security` コマンドも実行しない。

## 層の分け方

| 層 | 置き場所 | 役割 | import してよい層 |
|---|---|---|---|
| cli | `internal/cli/` | cobra のコマンド定義、フラグの解釈、出力の整形、終了コードへの変換 | cli、usecase、domain |
| usecase | `internal/usecase/` | 直下に `ports.go`（repository が満たすインターフェース）と `remote.go`（YouTube への接続）。操作の手順はサブパッケージ（`account/`・`setup/`・`video/`・`playlist/`）に1操作1ファイルで置く。偽物は `usecasetest/` | usecase、domain |
| domain | `internal/domain/` | 外部に触れない処理。直下はどの層も使うもの（エラーの種類、動画や再生リストの項目）。個別の決まりはサブパッケージ（`policy/`・`gcp/`・`configdir/`・`winpath/`・`video/`） | domain |
| repository | `internal/repository/` | 外部とのやりとり。相手ごとのサブパッケージ（`auth/`・`gcloud/`・`config/`・`browser/`・`atomicfile/`・`keychain/`・`terminal/`・`localfile/`・`youtubeapi/`）に置く。`localfile/` は上げる動画とサムネイルのファイルを開く、`youtubeapi/` は YouTube Data API | repository、usecase（ports）、domain |

- 入口は `cmd/youtube-cli/main.go` の1つだけ。各層を組み立てるだけにする。
- usecase・domain・repository は、層の中を関心ごとのサブパッケージに分ける。どのサブパッケージも使うものだけを層の直下に置き、直下からサブパッケージは import しない。新しいファイルは、関心の合うサブパッケージに置く。
- usecase のテストで使い回す偽物は `internal/usecase/usecasetest/` に置く（`_test.go` はパッケージをまたいで使えないため）。
- 依存の向きは `internal/archtest` と golangci-lint の depguard が確かめる（Google の API は repository だけ、cobra は cli と cmd だけ、`os/exec` は repository だけ）。

## タスクの管理（bd）

bd（beads）をステルスモードで使う。`.beads/` は git に入らない（`.git/info/exclude`）。自動バックアップは止めてある（`backup.enabled false`）。

- `bd ready`: 着手できるタスクを見る
- `bd create`: タスクを登録する（始める前に登録する）
- `bd update <id> --status in_progress`: 着手する
- `bd close <id>`: 終える

## テストの書き方

- `t.Run` の名前は日本語にする。
- 本文には `// Arrange` `// Act` `// Assert` の区切りを入れる。
- テストの値は、trivy-secret の規則（`GOCSPX-` に28文字が続く、`1//0` に40文字以上が続く）に当たらないものにする。

## 開発コマンド

- `make build` / `make install` / `make lint` / `make fmt` / `make test`
- `make build` は macOS で hardened runtime を付ける。`go build` のままではキーチェーンを使えない。`make install` は `~/.local/bin` に入れる（`BINDIR` で変えられる）。
- Windows 向けのコンパイルは `GOOS=windows go build ./...` と `GOOS=windows go vet ./...` で確かめる。Windows の上での振る舞いを確かめるのは利用者。
- コミット時は lefthook が整形・lint・テスト・trivy・commitlint を走らせる。
