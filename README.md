# youtube-cli

動画を YouTube へ限定公開（unlisted）だけで上げるための Go 製の CLI。

## 概要

Claude Code などの AI が作った動画を、自分の YouTube チャンネルへ限定公開で上げるために作った。人がターミナルで使うことも、AI エージェントやスクリプトから呼ぶことも想定している。

できること

- 動画の限定公開アップロード（`upload`）。タイトル・説明・タグ・サムネイル・再生リストへの追加も一度にできる
- このツールで上げた動画の一覧（`ls`）、サムネイルの設定（`thumbnail`）
- このツールで作る限定公開の再生リストの作成・一覧・動画の追加（`playlist`）

作りで守っていること

- **公開範囲は限定公開（`unlisted`）だけ。** `privacyStatus` に `unlisted` 以外を入れるコードがなく、機械的な検査（archtest）で確かめている。
- **できる操作を絞る。** 動画の削除・更新・公開範囲の変更の API は呼ばない（これも archtest が確かめる）。操作の対象は、目印のタグ `youtube-cli` を付けて、このツールで上げた動画と、説明の最後に目印の行 `[youtube-cli]` を入れて、このツールで作った再生リストだけ。
- **OAuth スコープは `https://www.googleapis.com/auth/youtube.force-ssl` だけ。** ただしこのスコープは広く、トークンが漏れると、Google 側では動画の削除や公開もできてしまう。この CLI にはそうした操作がないが、**スコープそのものが守っているわけではない。** トークンは暗号化して保存し、漏らさないことで守る（[docs/limitations.md](docs/limitations.md)）。
- 二重に上げない。同じタイトルの動画があれば止まる。結果が分からないときも、自動では送り直さない。

必要なもの: Google アカウントと YouTube チャンネル、ブラウザ、Go 1.27.3 以上。macOS を主に想定している（Linux、Windows 10・11 でも動く）。macOS では Xcode Command Line Tools も要る。詳しくは [docs/install.md](docs/install.md)。

## gcloud のインストール

`youtube-cli setup` が、GCP のプロジェクトを作って YouTube Data API を有効にするために gcloud CLI を使う。

macOS（Homebrew）:

```bash
brew install --cask gcloud-cli
```

Windows（PowerShell。[Scoop](https://scoop.sh/) を使う）:

```powershell
scoop bucket add extras
scoop install gcloud
```

Scoop を使わないときは、公式のインストーラー（https://cloud.google.com/sdk/docs/install ）で入れる。

Linux（ほかの入れ方は公式のページ: https://cloud.google.com/sdk/docs/install ）:

```bash
curl https://sdk.cloud.google.com | bash
exec -l $SHELL
```

入れたら、GCP のプロジェクトを作るアカウントでログインしておく。

```bash
gcloud auth login
```

GCP のプロジェクトをコンソールで手作業で用意する場合は、gcloud は要らない（[docs/setup.md](docs/setup.md)）。

## youtube-cli のインストール

macOS: リポジトリを clone して `make install` を実行する。`~/.local/bin` に入る（`make install BINDIR=/usr/local/bin` のように入れ先を変えられる）。

```bash
git clone https://github.com/wadoyoka/youtube-cli.git
cd youtube-cli
make install
```

`~/.local/bin` が PATH に入っていることを確かめる。macOS では `go install` や `go build` で入れたものは OS のキーチェーンを使えないので、必ず `make install` で入れる。

Linux: macOS と同じく、clone して `make install` を実行する（`~/.local/bin` に入る）。

```bash
git clone https://github.com/wadoyoka/youtube-cli.git
cd youtube-cli
make install
```

リポジトリは private なので、clone には GitHub の認証が要る。`go install …@latest` は、`GOPRIVATE` の設定と git の認証がないと使えない（[docs/install.md](docs/install.md)）。

Windows: PowerShell で、Scoop で git・go・make を入れてから、`make install` を実行する。`%USERPROFILE%\.local\bin\youtube-cli.exe` に入る。make は PowerShell、cmd、Git Bash のどこから実行してもよい。

```powershell
scoop install git go make
git clone https://github.com/wadoyoka/youtube-cli.git
cd youtube-cli
make install
[Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path", "User") + ";$env:USERPROFILE\.local\bin", "User")
```

最後の行は、PATH に `%USERPROFILE%\.local\bin` を足す（1回だけ）。足したら、端末を開き直す。

入れ方ごとの注意は [docs/install.md](docs/install.md)。

## プロジェクトのセットアップ

`youtube-cli setup` が、GCP プロジェクトの用意からログインまでを対話で案内する。済んでいる手順は飛ばすので、途中で止まっても同じコマンドでやり直せる。

```bash
youtube-cli setup
```

1. プロジェクト ID を聞かれる。Enter だけなら `youtube-cli-xxxxxx` という ID で作る。続けて YouTube Data API を有効にする。
2. OAuth の設定は公開 API がないので、ブラウザで手作業になる。`setup` が、プロジェクト ID 入りの URL で次の手順を表示し、ブラウザで開く。
   1. 「ブランディング」で、アプリ名とサポート用メールアドレスを設定する。
   2. 「対象（Audience）」で、ユーザーの種類を「外部」にする。公開ステータスは「テスト」のままにし、「テストユーザー」に、この CLI で YouTube を使う Google アカウント（自分を含む）を追加する。テストユーザーでないアカウントはログインできない。
   3. 「データアクセス」で `https://www.googleapis.com/auth/youtube.force-ssl` を追加する。
   4. 「クライアント」で、種類が「デスクトップアプリ」のクライアントを作り、JSON をダウンロードする。
3. ダウンロードした JSON のパスを入れる。暗号化して取り込まれる。
4. ブラウザで Google にログインし、上げ先のチャンネルを選ぶ。「Google はこのアプリを確認していません」と出たら、「続行」で進む。

**公開ステータスが「テスト」なので、ログインは 7日で切れる。** 切れると、コマンドが終了コード 3 で止まり、`youtube-cli login` のやり直しを求める。そのときは `youtube-cli login` を実行する（JSON の指定は要らない）。

**ダウンロードした JSON は、作成直後の画面でしか手に入らない。** パスワードマネージャーなど安全な場所に保管しておく（別の PC で使うときや、テストユーザーに入れたほかの人に使ってもらうときに要る。[docs/setup.md](docs/setup.md)）。リポジトリには入れない。

終わったら、ログインできていることを確かめる。

```bash
youtube-cli status
```

macOS では、最初の実行でキーチェーンのダイアログが出ることがある。ダイアログに youtube-cli と書いてあるときだけ「常に許可」を押す（[docs/setup.md](docs/setup.md#鍵の置き場所)）。

`setup` のフラグ、`init` と `login` を個別に使う方法、ブラウザのない環境での使い方は [docs/setup.md](docs/setup.md) と [docs/headless.md](docs/headless.md)。

## 動画のアップロード

```bash
youtube-cli upload ./intro.mp4 --title "【紹介動画】#.1 はじめに" --description-file ./desc.txt
youtube-cli upload ./intro.mp4 --title "【紹介動画】#.1 はじめに" --thumbnail ./thumb.png
youtube-cli ls -l                      # 上げたものを確かめる
```

結果に、動画のリンク（`https://youtu.be/...`）と YouTube Studio のリンクが出る。公開範囲は常に限定公開で、リンクを知っている人だけが見られる。

- タイトルを省くと、拡張子を除いたファイル名になる。100 文字以内で、`<` と `>` は使えない（説明も同じ）。
- 同じタイトルの、このツールで上げた動画があると、何も送らずに終了コード 5 で止まる。
- 途中で失敗したときや、サムネイルや再生リストの追加だけが失敗したときは、同じコマンドに `--skip-existing` を付けてやり直す。上げずに、既にある動画で残りの手順だけを行う。ただし「手元の記録に足せませんでした」と警告が出ていたときは、エラーに出る `youtube-cli thumbnail` か `youtube-cli playlist add` のコマンドでやり直す（[docs/commands.md](docs/commands.md#やり直し--skip-existing)）。
- 「アップロードの結果が分かりません」（終了コード 1）と出たときは、**自動では送り直さない。** YouTube Studio に動画があれば、`youtube-cli ls --json` に出るまで待ってから、同じコマンドに `--skip-existing` を付けてやり直す（出る前に付けると、もう1本上がる）。
- 1 日に上げられるのは 100 本まで（クォータ。終了コード 8）。
- 再生リストは、既定では拒否リストで止めてある。使うときは人が `policy.toml` を直す（[docs/policy.md](docs/policy.md)）。

## もっと詳しく

| 内容 | 文書 |
|---|---|
| 各コマンドのフラグと動き、JSON の形、終了コード | [docs/commands.md](docs/commands.md) |
| インストールの詳細 | [docs/install.md](docs/install.md) |
| セットアップの詳細、設定ディレクトリ、鍵の置き場所、すべてを消す方法 | [docs/setup.md](docs/setup.md) |
| 拒否リスト（`policy.toml`） | [docs/policy.md](docs/policy.md) |
| AI エージェントから使うとき | [docs/ai-agents.md](docs/ai-agents.md) |
| ブラウザやキーチェーンのない環境 | [docs/headless.md](docs/headless.md) |
| SSH で Mac に入って、切れたログインを更新する | [docs/faqs/ssh-relogin.md](docs/faqs/ssh-relogin.md) |
| 既知の制約、作らないもの | [docs/limitations.md](docs/limitations.md) |
| 開発 | [docs/development.md](docs/development.md) |
| CI、依存の更新（Renovate）、脆弱性の検査（trivy）、リリース | [docs/ci.md](docs/ci.md) |

## AI エージェント用のスキル

Claude Code と Codex に、youtube-cli の使い方（ログインの確かめ方、`--json` の読み方、止まるべき場面、タイトルの付け方）を教えるスキルを [agents/](agents/README.md) で配っている。

```bash
./agents/setup.sh
```
