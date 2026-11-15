# ブラウザのない環境

SSH の接続先、コンテナ、クラウドのサンドボックスなど、ブラウザやキーチェーンのない環境では、次のどちらかを使う。

キーチェーンに鍵を置いて使っている Mac に SSH で入り、切れたログインを更新するだけなら、[faqs/ssh-relogin.md](faqs/ssh-relogin.md) の手順でよい（キーチェーンのまま使える）。

## `login --no-browser`

キーチェーンのない環境では、鍵をファイルに置くよう `YOUTUBE_CLI_KEYRING=file` を付ける。

```bash
youtube-cli init
YOUTUBE_CLI_KEYRING=file youtube-cli login --no-browser --client-secret <JSON>
```

以後のコマンドでも、同じ環境変数を付ける。

1. CLI がログイン用の URL を表示する。
2. その URL を、別の PC のブラウザで開いてログインする。
3. ログイン後、ブラウザは `127.0.0.1` に飛ばされ、「接続できません」と表示される。そのときのアドレスバーの URL を丸ごとコピーして、CLI の標準入力に貼り付ける。
4. CLI がその URL から認可コードと `state` を取り出し、トークンに交換して、暗号化して保存する。

**他人から渡された URL は貼り付けない。** 自分でログインして、自分でアドレスバーからコピーした URL だけを貼る。他人が自分のアカウントでログインして得た URL を貼ると、CLI はその人の YouTube チャンネルにログインした状態になる。そのあとのアップロードは、その人のチャンネルに入る。`state` が合わなければ CLI は止まるが、ログイン用の URL が相手に見えていれば、相手は同じ `state` を使える。

## 設定ディレクトリのコピー

ログイン済みの設定ディレクトリ（`~/.config/youtube-cli/`。Windows では `%LOCALAPPDATA%\youtube-cli\`）を別の環境へコピーし、`YOUTUBE_CLI_CONFIG_DIR` で指定する。

- 鍵を OS のキーチェーンに置いている場合、設定ディレクトリをコピーしても、コピー先には鍵がないので使えない。これは暗号化の目的どおりの動き。
- コピーして使うなら、コピー元でも `YOUTUBE_CLI_KEYRING=file` でログインし、`key` ごとコピーする。コピー先でも `YOUTUBE_CLI_KEYRING=file` を付ける。

```bash
scp -rp ~/.config/youtube-cli remote:/home/me/youtube-cli-config
ssh remote 'chmod 700 /home/me/youtube-cli-config && chmod 600 /home/me/youtube-cli-config/*'
ssh remote 'YOUTUBE_CLI_KEYRING=file YOUTUBE_CLI_CONFIG_DIR=/home/me/youtube-cli-config youtube-cli status'
```

`scp -p` で権限を保ったうえで、コピー先でもディレクトリを 0700、中のファイルを 0600 にそろえる（`key` も含まれるため）。

トークンと鍵（`token.enc`、`key`）を含むので、渡す先は信頼できる環境だけにする。
