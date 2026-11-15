# インストールの詳細

手順の要点は [README](../README.md#youtube-cli-のインストール) にある。ここでは、必要なものと、入れ方ごとの注意を書く。

## 必要なもの

- Go 1.27.3 以上（`go install` や `make install` で入れる場合）
- Google アカウントと、そのアカウントの YouTube チャンネル（個人のもの。Workspace ではない）
- gcloud CLI（`youtube-cli setup` が GCP のプロジェクトを作るために使う。手作業で設定する場合は不要）
- ブラウザ（`login` のため。ない環境は [headless.md](headless.md) を参照）
- macOS を主に想定している。Linux、Windows 10・11 でも動く。トークンの暗号化の鍵は OS のキーチェーンに置く（キーチェーンのない環境は [setup.md](setup.md#鍵の置き場所) を参照。Windows は資格情報マネージャー）。
- macOS では Xcode Command Line Tools（cgo と `codesign` のため）。
- Windows では git、go、make（Scoop で入れられる。[README](../README.md#youtube-cli-のインストール)）。

## macOS

リポジトリを clone して `make install` を実行する。`~/.local/bin` に入る（`make install BINDIR=/usr/local/bin` のように入れ先を変えられる）。

```bash
git clone https://github.com/wadoyoka/youtube-cli.git
cd youtube-cli
make install
```

`~/.local/bin` が PATH に入っていることを確かめる（入っていなければ PATH に足すか、`BINDIR` で PATH の通ったディレクトリを指定する）。

`make install` は、実行ファイルに hardened runtime を付ける（証明書の要らない ad-hoc 署名）。macOS で `go install` や `go build` で入れた youtube-cli は、hardened runtime が付かないので、OS のキーチェーンを使えない（終了コード 3 で止まる）。その場合は `YOUTUBE_CLI_KEYRING=file` を指定するか、`make install` で入れ直す。

新しい版を入れたあとの最初の実行では、キーチェーンのダイアログが出る。許可してよい相手は [setup.md](setup.md#鍵の置き場所) を参照。

### 前の版を `go install` で入れていた場合

その古い実行ファイルを消す（`$(go env GOPATH)/bin` か、使っているバージョン管理ツールの Go の bin ディレクトリにある）。そのあと `which -a youtube-cli` を実行して、`make install` で入れたものだけが並ぶことを確かめる。古い実行ファイルは古い（守りのない）項目を使い続け、移し替えのあとに動くと、`security` に対するダイアログが出るため。

## Linux

```bash
go install github.com/wadoyoka/youtube-cli/cmd/youtube-cli@latest
```

`$(go env GOPATH)/bin` が PATH に入っていることを確かめる。鍵は Secret Service に置く。Secret Service のない環境では `YOUTUBE_CLI_KEYRING=file` を使う（[setup.md](setup.md#鍵の置き場所)）。

## Windows

README の手順で入れる（PowerShell で `scoop install git go make`、clone、`make install`、PATH に `%USERPROFILE%\.local\bin` を足す）。`make install` は `go build` だけで `%USERPROFILE%\.local\bin\youtube-cli.exe` に入れる（`make install BINDIR=...` で入れ先を変えられる）。make は PowerShell、cmd、Git Bash のどこから実行してもよい。

- 動いている `youtube-cli.exe` は上書きできない。`make install` が失敗したら、動いている youtube-cli（別の端末や AI エージェントのものを含む）を止めてから、やり直す。
- PowerShell で youtube-cli の出力を変数や別のコマンドに渡したときに日本語が化けたら、先に `[Console]::OutputEncoding = [Text.Encoding]::UTF8` を実行する。
- `make help` の表示が化けたら、先に `chcp 65001` を実行する。
