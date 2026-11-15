# youtube-cli コマンドの早見表（エージェント向け）

すべてのコマンドに `--json` を付けて使う。`setup`・`login`・`logout`・`init` は人が実行するので、ここには載せない。

## 共通

- 成功時: 標準出力に結果の JSON が 1 つ（1 行）出る。
- 失敗時: 標準エラーに次の JSON が出る。原因（YouTube API や通信のエラー）は `details` の最後の要素に入る。

  ```json
  {"error":{"kind":"conflict","message":"...","details":["..."]}}
  ```

- `upload` で動画ができたあとにサムネイルや再生リストの追加が失敗したときは、結果が標準出力に、エラーが標準エラーに、と両方に出る。
- 対象は、タグ `youtube-cli` を付けてこのツールで上げた動画と、説明の最後の行が `[youtube-cli]` の、このツールで作った再生リストだけ。

## 終了コード

| 終了コード | `kind` | 意味 | エージェントのすること |
|---|---|---|---|
| 0 | | 成功 | 結果を人に伝える |
| 1 | `unexpected` | 予期しないエラー。「アップロードの結果が分かりません」、または動画ができたあとのサムネイル・再生リストの失敗 | 「結果が分かりません」なら止まって人に伝える（送り直さない）。後者なら `--skip-existing` でやり直す |
| 2 | `usage` | 引数の誤り | コマンドを直す |
| 3 | `setup` | 準備が必要（ログインしていない、鍵がない、キーチェーンが使えないなど） | 止まって人に戻す |
| 4 | `not_found` | 手元のファイル、動画、再生リストが見つからない | 名前や ID を確かめる |
| 5 | `conflict` | 同じタイトルの動画がある、同じ名前の再生リストがある、使えない状態の既存の動画がある | タイトルを見直す。使えない状態なら止まって人に伝える（Studio で消してもらう） |
| 6 | `ambiguous` | 再生リストの名前が複数ある | 候補を人に見せて判断を仰ぐ。推測で選ばない |
| 7 | `denied` | 拒否リスト（`policy.toml`）で止められた。動画を入れようとした再生リストが限定公開でない場合も含む | 止まって人に伝える。回避しない。再生リストなら、再生リスト抜きで進めて人に伝える |
| 8 | `quota` | クォータを使い切った（動画は 1 日 100 本まで） | 止まって人に伝える。太平洋時間の 0 時に戻る |

## status

```bash
youtube-cli status --json
```

```json
{"configDir":"...","loggedIn":true,"keyStore":"os","channelId":"UC...","channelTitle":"...","channelUrl":"https://www.youtube.com/channel/UC..."}
```

ログインしていないときは `loggedIn` が `false` で、`problem` に理由が入り、終了コードは 3。
キーチェーンが使えないときは、理由は標準エラーのエラーの JSON の `message` に出る。

## upload

```bash
youtube-cli upload --json [--title <タイトル>] [--description <説明> | --description-file <ファイル>] [--tag <タグ>]... \
  [--category <ID>] [--made-for-kids] [--synthetic-media] [--thumbnail <画像>] [--playlist <名前か ID>] [--skip-existing] <動画>
```

- `--title` を省くと、拡張子を除いたファイル名。100 文字以内で、`<` と `>` は使えない。説明は 5000 バイト以内。タグの合計は 500 文字以内。目印のタグ `youtube-cli` は自動で足す。
- `--category` の既定は `22`。
- `--synthetic-media` は、本物と見分けがつかない人物や出来事の合成・改変のコンテンツのときだけ付ける。
- `--thumbnail` は jpg か png で 2 MiB まで。チャンネルの電話番号の確認が要る。`--playlist` は、このツールで作った限定公開の再生リスト。
- 拒否リストは、`upload` に加えて、`--thumbnail` で `thumbnail`、`--playlist` で `playlist-add` も確かめる。
- 上げる前に、項目・動画・サムネイル・再生リストを確かめる。ここで失敗すれば、何も上げない。
- 同じタイトルの、このツールで上げた動画があると、何も上げずに終了コード 5。`--skip-existing` を付けると、上げずにそれを使い、サムネイルと再生リストの手順だけを行う（`existing` が `true`）。
  既存の動画が失敗・却下・削除の状態なら、`--skip-existing` でも終了コード 5。人が Studio で消す。
- 16 MiB より小さい動画は 1 回の要求で、それ以上は分けて送る。送り始めたあとで動画 ID を受け取れなかったときは、終了コード 1 で「アップロードの結果が分かりません」と出る。**自動では送り直さない。** Studio で確かめてから `--skip-existing`。

結果:

```json
{"videoId":"abc123","title":"...","url":"https://youtu.be/abc123","studioUrl":"https://studio.youtube.com/video/abc123/edit","privacyStatus":"unlisted","uploadStatus":"uploaded","existing":false,"thumbnail":null,"playlist":null,"warnings":[]}
```

- `thumbnail` と `playlist` は、そのフラグを付けたときだけ `{"done":true}`（失敗なら `{"done":false,"error":"..."}`）。付けなければ `null`。
- `privacyStatus` が `unlisted` でなければ、`warnings` に出る。

## ls

```bash
youtube-cli ls --json
youtube-cli ls --json --playlist "シリーズ名"
```

結果は、公開日時の新しい順の配列。

```json
[{"id":"abc123","title":"【シリーズ】#.1 ...","url":"https://youtu.be/abc123","privacyStatus":"unlisted","uploadStatus":"processed","publishedAt":"2026-11-15T00:00:00Z","tags":["youtube-cli"]}]
```

- `--playlist` で、このツールで作ったその再生リスト（名前か ID）に入っている動画だけにする。なければ終了コード 4、名前が複数なら 6。
- この PC で上げた動画はすぐ出る。ほかの PC で上げた直後の動画は、遅れて出ることがある。

## thumbnail

```bash
youtube-cli thumbnail --json <動画 ID> <画像>
```

結果は `{"videoId":"...","url":"..."}`。このツールで上げた動画だけが対象（なければ終了コード 4）。

## playlist

拒否リストの名前は `playlist-ls`・`playlist-create`・`playlist-add`。既定では `playlist-create` と `playlist-add` が止まっている。

```bash
youtube-cli playlist ls --json
youtube-cli playlist create --json "シリーズ名" [--description <説明>]
youtube-cli playlist add --json <動画 ID> <再生リストの名前か ID>
```

- `playlist ls`: 配列。各要素は `{"id":"...","title":"...","url":"...","privacyStatus":"unlisted","itemCount":3}`。
- `playlist create`: 限定公開の再生リストを作る（説明の最後に `[youtube-cli]` を足す）。同じ名前があれば終了コード 5。結果は `{"id":"...","title":"...","url":"...","privacyStatus":"unlisted","warnings":[]}`。
- `playlist add`: このツールで上げた動画を、このツールで作った限定公開の再生リストに入れる。結果は `{"videoId":"...","playlistId":"...","playlistTitle":"...","playlistUrl":"...","added":true}`（`added` が `false` ならすでに入っていた）。
  再生リストが限定公開でなければ終了コード 7。
