# 拒否リスト（`policy.toml`）

`init` が設定ディレクトリに作るファイル。ここに書いたコマンドは、終了コード 7 で止まる。人が手で編集する。

```toml
# youtube-cli で実行を拒否するコマンドの一覧。
# ここに書いたコマンドを実行しようとすると、終了コード 7 で止まる。
# 書ける名前: setup, login, logout, status, upload, thumbnail, ls, playlist-ls, playlist-create, playlist-add
# upload --thumbnail は thumbnail も、upload --playlist は playlist-add も確かめる。
# 再生リストを使うときは、下の playlist-create と playlist-add を消す。
deny = ["playlist-create", "playlist-add"]
```

**初期値は `deny = ["playlist-create", "playlist-add"]`。** 再生リストを作ることと、動画を入れることは、既定では止めてある。使うときは、人がこの 2 つを消す。

- `upload --thumbnail` は、`upload` に加えて `thumbnail` も確かめる。`thumbnail` が拒否されていれば、何も上げずに終了コード 7 になる。
- `upload --playlist` は、`upload` に加えて `playlist-add` も確かめる。
- `playlist ls` は `playlist-ls`、`playlist create` は `playlist-create`、`playlist add` は `playlist-add`。`ls --playlist` は、`ls` だけを確かめる。
- 例えば AI エージェントに読み取りだけをさせたいときは、`deny = ["upload", "thumbnail", "playlist-create", "playlist-add", "login", "logout"]` のようにする。

**安全側に倒す（fail closed）。** 誤字があったときに、拒否したつもりが素通りになるのを防ぐための規則。

| 状態 | 結果 |
|---|---|
| ファイルがない | 終了コード 3。`youtube-cli init` を実行する |
| 読めない、または TOML として正しくない | 終了コード 7。すべてのコマンドが止まる |
| 知らないキーがある（例: `denny`） | 終了コード 7。すべてが止まる |
| 知らないコマンド名がある（例: `uplaod`） | 終了コード 7。すべてが止まる |

- `init` と `help` は、拒否リストの対象にならない。拒否リストを作る、または直すためのコマンドだから。`setup` はルートの判定から外れていて、途中で `policy.toml` を作ったあとに、自分で `setup` と `login` の両方が拒否されていないかを調べる（`setup` の中でログインもするため）。`playlist` だけ（子コマンドなし）も、使い方を出すだけなので対象にならない。それ以外のすべてのコマンドは、実行の前に拒否リストを読む。
- 拒否リストは、誤操作を防ぐための柵であって、セキュリティの境界ではない。設定ディレクトリに書き込める人やプログラム（AI エージェントを含む）は書き換えられる。本当の守りは、公開範囲を限定公開に固定し、削除・更新・公開範囲の変更の API を呼ばない作りである（機械的な検査で守っている）。ただし OAuth スコープ（`youtube.force-ssl`）は広いので、トークンが漏れたときまでは守れない（[limitations.md](limitations.md)）。
