# CI・依存の更新・リリース

GitHub Actions のワークフローは `.github/workflows/` にある。

| ワークフロー | 動くとき | すること |
|---|---|---|
| `ci.yml` | PR、main と dev への push、手動 | commitlint（PR のときだけ）、golangci-lint（Linux と Windows 向け）、`go test`、trivy。Windows の上のテスト（go-windows）は手動のときだけ |
| `trivy-daily.yml` | 毎日 9:00（JST）、手動 | trivy で調べて Slack に知らせる。直った版が出ている脆弱性は、上げて dev 向けの PR にする |
| `renovate.yml` | 毎週金曜 9:00（JST）、手動 | Renovate で依存の新しい版を探し、dev 向けの PR にする |
| `renovate-notify.yml` | Renovate が PR を作ったとき | Slack に知らせる |
| `release.yml` | main への push | semantic-release でタグと GitHub のリリースを作る |

変更は、作業ブランチ → dev → main の順に入れる。Renovate と trivy-daily が作る PR も dev に向ける。

schedule で動くワークフローと `.github/renovate.json5` は、既定のブランチにあるものが使われる。既定のブランチを main にしたときは、これらを変えても、dev を main に入れるまで効かない。

## 脆弱性の検査（trivy）

```bash
trivy fs .   # 依存の既知の脆弱性（HIGH 以上）と、秘密情報の混入を調べる
```

設定は `trivy.yaml`（対象・重大度・除外するディレクトリ）と `trivy-secret.yaml`（Google の OAuth のクライアントシークレットとリフレッシュトークンの規則）にある。lefthook、`ci.yml`、`trivy-daily.yml` が同じ設定で動かす。

`trivy-daily.yml` は、毎日 9:00（JST）に既定のブランチを調べ、結果を Slack（Secrets の `SLACK_WEBHOOK_URL`）に送る。見つかったときは、送ったあとでワークフローを失敗させ、Actions の画面でも赤く見えるようにする。

直った版が出ている脆弱性は、同じワークフローの `fix` ジョブが上げて、dev 向けの PR（ブランチ `trivy/fix-vulnerabilities`）にする。

- PR は dev に向けるので、dev を調べる。dev で直っていれば PR は作らない。
- npm の依存は `pnpm update --no-save` で、package.json の範囲の中の新しい版に上げる。Go のモジュールは `go get` で、直った版のうち一番古い版に上げる（go.mod の `// indirect` の依存も）。どちらも Renovate と同じく、公開から 3 日経った版だけを使う。取り下げられた（retract）Go のモジュールの版は使わない。
- 直った版が、いまより新しい Go を求めるモジュールは上げない（`GOTOOLCHAIN=local` で `go get` が失敗する）。go.mod の `go` の行と `.mise.toml` の go がずれないようにするため。Go 本体の版は、Renovate の go のグループで上げる。
- 上げられなかったものは、PR の「残っているもの」に並ぶ。手で直す。
- youtube-cli の実行ファイルに入る依存（Go のモジュール）を 1 つでも直したら `fix: :bug:`（main に入るとパッチ版が出る）、package.json の開発用の依存だけなら `chore: :broom:` にする。trivy は Go のモジュールを開発用と見分けないので、テストだけが使う golang.org/x/tools を直したときも `fix` になる。リリースしたくないときは、コミットの type を直す。
- PR は 1 つだけで、毎日その日の dev から作り直す。このブランチに足したコミットは消えるので、手で直すときは別のブランチで直す。dev で直って差分がなくなると、PR を閉じてブランチを消す。
- PR は Renovate と同じ GitHub App のトークンで作る（「[最初の準備](#最初の準備)」）。トークンの権限は、Contents と Pull requests の書き込みだけに絞る。
- 外のアクションは使わず、`gh` で GitHub の API を直接呼んで、App の署名付きコミットと PR を作る。
- PR ができたときと、失敗したときに Slack に知らせる。

## 依存の更新（Renovate）

`renovate.yml` が毎週金曜の 9:00（JST）に [Renovate](https://github.com/renovatebot/renovate) を動かし、依存に新しい版があれば dev 向けの PR を作る。Mend のクラウド版（Mend Renovate App）は使わず、Renovate の Docker イメージをこのリポジトリの GitHub Actions で動かす。何をどう上げるかは `.github/renovate.json5` に書いてある。

- 対象は `go.mod`（require のモジュールと `go` の行）、`package.json` と `pnpm-lock.yaml`、`.github/workflows/` のアクション、`.mise.toml` のツール。go.mod の `// indirect` の依存は上げない（脆弱性があれば trivy-daily が上げる）。`trash/go.mod` も上げない。
- 公開から 3 日経った版だけを PR にする。待っている更新や、数の上限でまだ PR にしていない更新は、Renovate が作る issue「Dependency Dashboard」に並ぶ。
- 確かめ方が同じ依存を、1 つの PR にまとめる。

| グループ | 中身 | 確かめ方 |
|---|---|---|
| go modules | go.mod の require（下の google api と keyring を除く） | CI。golang.org/x/sys と golang.org/x/term は Windows の分でも使うので、`ci.yml` を Run workflow で起動して go-windows も通す |
| google api | google.golang.org/api、golang.org/x/oauth2 など | CI と E2E テスト（[development.md](development.md#e2e-テスト)） |
| keyring | github.com/zalando/go-keyring | CI と、macOS のキーチェーン・Windows の資格情報マネージャーでの読み書き（利用者が確かめる） |
| go | `.mise.toml` の go と go.mod の `go` の行 | CI |
| toolchain | `.mise.toml` のツール（go を除く）と package.json の packageManager | `mise install` し直して、CI |
| commit and release tools | package.json の devDependencies（commitlint・semantic-release） | CI の commitlint。semantic-release は main に入れたあとのリリースで |
| github actions | `.github/workflows/` のアクション | ワークフローが動くこと。`release.yml` と `trivy-daily.yml` は PR では動かないので、main に入れたあとに |
| renovate | Renovate 本体 | 毎日のように新しい版が出るので、月の最初の金曜にだけ上げる |

- major の更新は、グループごとに別の PR（タイトルの末尾が (major)）になる。npm・mise・Actions の 0.x の minor の更新も、壊れることがあるので依存ごとの PR になる。Go のモジュールは 0.x でもグループのまま。
- Go のモジュールは、go.mod の `go` の行の Go で使える版だけを PR にする。新しい Go が要る版は、go のグループで Go を上げたあとに PR になる。
- コミットメッセージと PR タイトルは commitlint の形になる。youtube-cli の実行ファイルに入るもの（go.mod の require と Go 本体）は `fix: :bug:` で、main に入るとパッチ版が出る。それ以外（golang.org/x/tools・開発用ツール・アクション・mise のツール）は `chore: :broom:` で、リリースしない。
- PR ができると `renovate-notify.yml` が Slack（`SLACK_WEBHOOK_URL`）に知らせる。Renovate の実行が失敗したときも知らせる。
- 手で動かすときは、Actions の `renovate` ワークフローの **Run workflow** を使う。`dry-run` に印を付けると、ブランチと PR を作らずに、何をするかをログに出すだけになる。

## リリース（semantic-release）

dev を main にマージすると、`release.yml` が [semantic-release](https://github.com/semantic-release/semantic-release) を動かす。前のタグからのコミットの type で次の版を決め、タグ（`v1.2.3`）と GitHub のリリース（リリースノート）を作る。設定は `release.config.mjs`。

| コミット | 次の版 |
|---|---|
| `feat` | マイナー版を上げる（1.2.3 → 1.3.0） |
| `fix`・`perf`・`revert` | パッチ版を上げる（1.2.3 → 1.2.4） |
| 本文に `BREAKING CHANGE:` がある | メジャー版を上げる（1.2.3 → 2.0.0） |
| それ以外（docs・refactor・test・build・ci・chore など）だけ | リリースしない |

- 最初のリリースは v1.0.0 になる。
- 実行ファイルのビルドと配布（CD）はしない。リリースにはリリースノートだけが載る。版を書き込むファイルもないので、版のコミットは作らない。
- リリースノートには、新機能（feat）・不具合の修正（fix）・速さの改善（perf）・取り消し（revert）の見出しでコミットが並ぶ。
- タグは Go のモジュールの版にもなる。v2 以降のタグを Go のモジュールとして使ってもらうには、go.mod の module を `…/v2` にしておく必要がある。BREAKING CHANGE を入れるときは、先に module のパスを変える。
- PR や issue にコメントやラベルは付けない。失敗しても issue は作らない。
- トークンは Actions の `GITHUB_TOKEN`（`contents: write`）を使う。App もほかの Secrets も要らない。

## 最初の準備

### ブランチ

GitHub に main と dev を作る。Renovate と trivy-daily の PR は dev に向くので、dev がないと PR を作れない。

### GitHub App

Renovate は、このリポジトリ専用の GitHub App のトークンで PR を作る。Actions 標準の `GITHUB_TOKEN` で作った PR では `ci.yml` などのワークフローが動かず、`.github/workflows/` のファイルも書き換えられないため。trivy-daily の `fix` ジョブも、同じ App のトークンで PR を作る（下の権限で足りる）。

1. 自分のアカウントの **Settings → Developer settings → GitHub Apps → New GitHub App**（<https://github.com/settings/apps/new>）で App を作る。
   - URL の欄は次のようにする。
     - **Homepage URL**（必須）: このリポジトリの URL を入れる。App のページにリンクとして出るだけで、動作には使われない。
     - **Callback URL**: 空のままにする。**Request user authorization (OAuth) during installation** にも印を付けない。Renovate は人としてログインせず、App として動くため。
     - **Setup URL**: 空のままにする。
     - **Webhook**: **Active** を外す。外すと Webhook URL を入れなくてよくなる。Renovate は cron で決まった時刻に自分から見に行くので、GitHub からの知らせを受け取るサーバーは要らない。ここに Slack の Webhook URL は入れない。
   - 権限は [Renovate のドキュメント](https://docs.renovatebot.com/modules/platform/github/#running-as-a-github-app)のとおりにする。ただし Merge queues だけは Read-only にする（理由は表の下）。表にない権限は、Account permissions も含めて No access のままにする。

     | Repository permissions | Access |
     |---|---|
     | Administration | Read-only |
     | Checks | Read and write |
     | Commit statuses | Read and write |
     | Contents | Read and write |
     | Dependabot alerts | Read-only |
     | Issues | Read and write |
     | Merge queues | Read-only |
     | Metadata | Read-only |
     | Pull requests | Read and write |
     | Workflows | Read and write |

     Renovate のドキュメントは Merge queues を Read and write にするよう勧めているが、書き込みはマージキューに PR を入れるときにしか使わない。個人のアカウントのリポジトリではマージキューを使えないので、その場面がない。一方、読み込みは付けておく。Renovate は毎回最初に、リポジトリの情報と一緒にマージキューがあるかを読み、この問い合わせがエラーになると、その回の処理をすべて止めるため。

   - **Where can this GitHub App be installed?** は **Only on this account** にする。
2. 作った App のページで **Generate a private key** を押し、`.pem` ファイルをダウンロードする。リポジトリには入れない。
3. **Install App** から自分のアカウントにインストールし、**Only select repositories** でこのリポジトリだけを選ぶ。
4. このリポジトリの **Settings → Secrets and variables → Actions** の Secrets に登録する。
   - `RENOVATE_APP_CLIENT_ID`: App のページにある Client ID（`Iv23` で始まる値。数字だけの App ID ではない）
   - `RENOVATE_APP_PRIVATE_KEY`: `.pem` ファイルの中身をまるごと
   - `SLACK_WEBHOOK_URL`: 知らせる先の Slack の Incoming Webhook の URL。登録していないと、Slack に知らせるステップが失敗する
