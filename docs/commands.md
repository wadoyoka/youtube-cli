# コマンドと使い方

共通のフラグは `--json`（結果を JSON で標準出力に出す。エラーも JSON にして標準エラーに出す）。普段は人が読みやすい表示になる。`setup` と `login` 以外のコマンドは、標準入力から何も読まず、入力を求めて止まらない。

| コマンド | 内容 |
|---|---|
| `init` | 設定ディレクトリと拒否リストを用意する |
| `setup [--project <ID>] [--client-secret <JSON>] [--no-browser]` | gcloud で GCP プロジェクトを用意し、手作業を案内して、ログインまで進める（人が端末で実行する） |
| `login [--client-secret <JSON>] [--no-browser] [--switch-channel]` | Google にログインし、チャンネルを確かめる |
| `logout` | Google 側でトークンを無効化し、保存したトークンを消す |
| `status` | ログイン状態、チャンネル、鍵の置き場所を表示する |
| `upload <動画> [フラグ]` | 動画を限定公開で上げる |
| `ls [-l] [--playlist <名前か ID>]` | このツールで上げた動画を新しい順に表示する |
| `thumbnail <動画 ID> <画像>` | このツールで上げた動画のサムネイルを設定する |
| `playlist ls` | このツールで作った再生リストを表示する |
| `playlist create <名前> [--description <説明>]` | 限定公開の再生リストを作る |
| `playlist add <動画 ID> <再生リストの名前か ID>` | 動画を再生リストに入れる |

`setup`、`init`、`login` は [setup.md](setup.md) を参照。

## 「このツールで上げた」の見分け方

動画は、タグ `youtube-cli`（上げるときに自動で足す）で見分ける。再生リストは、説明の最後の行 `[youtube-cli]`（作るときに自動で足す）で見分ける。ブラウザや Studio から入れた動画や、ほかの方法で作った再生リストは、`ls`・`thumbnail`・`playlist` の対象にならない。Studio でタグや説明の目印を消すと、対象から外れる。

## status

```bash
youtube-cli status
youtube-cli status --json
```

AI エージェントが作業前に確認するためのコマンド。鍵の置き場所（`os` か `file`）も表示する（`--json` では `keyStore`）。

```json
{"configDir":"...","loggedIn":true,"keyStore":"os","channelId":"UC...","channelTitle":"...","channelUrl":"https://www.youtube.com/channel/UC..."}
```

ログインしていないときは `loggedIn` が `false` で、`problem` に理由（鍵がない、合わない、古い形式のファイルだけがある）が入り、終了コードは 3。キーチェーンが使えないときは、理由は `problem` には入らず、エラーとして標準エラーに出る（`--json` では標準エラーのエラーの JSON の `message`）。どの場合も終了コードは 3。

## login

```bash
youtube-cli login --client-secret ~/Downloads/client_secret_xxx.json   # 初回
youtube-cli login                                                      # 2回目以降（7日で切れたとき）
youtube-cli login --switch-channel                                     # 上げ先のチャンネルを変える
```

- 結果は、ログインしたチャンネル（`--json` では `channelId`・`channelTitle`・`channelUrl`・`warnings`）。
- 保存してあるチャンネルと違うチャンネルを選ぶと、何も保存せず終了コード 5 で止まる。切り替えるときは `--switch-channel` を付ける。切り替えると、この PC の手元の記録（`ledger.json`）を消す（別のチャンネルのものだから）。警告として「チャンネルを〜に切り替えました」が出る。
- ログインでは、ブラウザで上げ先のチャンネルを選ぶ。いつも使うチャンネルを選ぶ。

## upload

```bash
youtube-cli upload ./intro.mp4
youtube-cli upload ./intro.mp4 --title "【紹介動画】#.2 使い方" --description-file ./desc.txt --tag 紹介 --tag 使い方
youtube-cli upload ./intro.mp4 --thumbnail ./thumb.png --playlist "紹介動画"
youtube-cli upload ./intro.mp4 --title "【紹介動画】#.2 使い方" --skip-existing --json
```

| フラグ | 意味 |
|---|---|
| `--title` | タイトル。省くと、拡張子を除いたファイル名。100 文字以内、`<` と `>` は使えない |
| `--description` | 説明。5000 バイト以内、`<` と `>` は使えない（`->` も不可。上げる前に終了コード 2 で止まる） |
| `--description-file` | 説明を読むファイル。`--description` と同時には指定できない（終了コード 2）。ファイルがなければ 4 |
| `--tag` | タグ。繰り返して複数付けられる。目印の `youtube-cli` は自動で足す。合計 500 文字以内 |
| `--category` | カテゴリの ID。既定は `22`（ブログと人々） |
| `--made-for-kids` | 子ども向けとして申告する |
| `--synthetic-media` | 改変・合成されたリアルなコンテンツとして申告する |
| `--thumbnail` | 上げたあとで設定するサムネイル。jpg か png、2 MiB まで |
| `--playlist` | 上げたあとで入れる再生リスト。このツールで作った限定公開のもの（名前か ID） |
| `--skip-existing` | 同じタイトルの、このツールで上げた動画があれば、上げずにそれを使い、残りの手順だけを行う |

動き

1. **上げる前にすべてを確かめる。** 項目、動画のファイル、サムネイルの形式と大きさ、再生リスト（このツールで作った限定公開のものか）を確かめる。ここで失敗すれば、何も上げない。
2. 同じタイトルの、このツールで上げた動画があれば、何も上げずに終了コード 5 で止まる（`--skip-existing` で変えられる。下の「やり直し」）。
3. 限定公開で上げる。16 MiB より小さい動画は 1 回の要求で、それ以上は分けて送る（再開可能アップロード）。
4. 動画 ID を受け取ったらすぐに、この PC の手元の記録（`ledger.json`）に書く。書けなくても、上げた動画は結果に出し、警告を付ける。
5. YouTube が限定公開以外で返してきたら、警告を出す（Studio で確かめる）。
6. `--thumbnail` があればサムネイルを設定し、`--playlist` があれば再生リストに入れる（すでに入っていれば何もしない）。片方が失敗してももう片方は行う。

拒否リストの確かめ: `upload` に加えて、`--thumbnail` を付けると `thumbnail`、`--playlist` を付けると `playlist-add` も確かめる（どれかが拒否されていれば、何も上げずに終了コード 7）。

### 結果

`--json` なし（標準出力に 1 行、タブ区切り）:

```
uploaded	<タイトル>	<動画の URL>	<Studio の URL>
```

`existing`（`--skip-existing` で既存の動画を使ったとき）の場合は、先頭が `existing`。警告は標準エラーに `注意: ...` と出る。

`--json`:

```json
{"videoId":"abc123","title":"...","url":"https://youtu.be/abc123","studioUrl":"https://studio.youtube.com/video/abc123/edit","privacyStatus":"unlisted","uploadStatus":"uploaded","existing":false,"thumbnail":{"done":true},"playlist":null,"warnings":[]}
```

- `thumbnail` と `playlist` は、そのフラグを付けたときだけオブジェクト（`done`、失敗したら `error` も）で、付けなければ `null`。
- `privacyStatus` は、`unlisted` でなければ警告が出る。
- 動画ができたあとで、サムネイルか再生リストの追加が失敗したときは、**結果の JSON が標準出力に、エラーの JSON が標準エラーに、と両方に出て、終了コードは 1** になる。動画は YouTube にある。`thumbnail.done` と `playlist.done` で、終わっていない手順を見分ける。

### やり直し（`--skip-existing`）

- 同じタイトルの動画があるのに `--skip-existing` を付けないと、終了コード 5。詳細に、その動画の ID・状態・Studio の URL が出る。
- `--skip-existing` を付けると、何も送らず、いちばん新しい既存の動画を使って、`--thumbnail` と `--playlist` の手順だけを行う。結果の `existing` が `true` になる。同じタイトルが複数あれば、警告を出していちばん新しいものを使う。
- 既存の動画が失敗（`failed`）・却下（`rejected`）・削除（`deleted`）の状態なら、`--skip-existing` を付けても終了コード 5 で止まる。「Studio でその動画を消してから、やり直してください」と出る。youtube-cli には削除がないので、Studio で消す。
- **結果が分からないとき。** 動画を送り始めたあとで動画 ID を受け取れなかったときは、終了コード 1 と「アップロードの結果が分かりません。Studio に動画があれば、`youtube-cli ls --json` に出るまで待ってから、同じコマンドに `--skip-existing` を付けてやり直してください（出る前に付けると、もう1本上がります）」と出る。YouTube に動画ができているかもしれないので、**自動では送り直さない。** 結果が分からない動画は手元の記録にないので、`--skip-existing` が見つけられるのは YouTube の一覧に出てから。Studio に動画があれば、`youtube-cli ls --json` に出るまで待ってから、同じコマンドに `--skip-existing` を付けてやり直す（出る前に付けると、もう1本上がる）。Studio になければ、そのまま同じコマンドをやり直す。
- **手元の記録に足せなかったとき。** 動画はできたのに、`warnings` に「上げた動画を手元の記録に足せませんでした」と出て、さらにサムネイルか再生リストの手順が失敗したときは、`--skip-existing` でやり直さない（一覧に出る前だと、もう1本上がる）。エラーに出る `youtube-cli thumbnail <動画 ID> <画像>` か `youtube-cli playlist add <動画 ID> <再生リスト>` で、失敗した手順だけをやり直す。この 2 つは動画を上げ直さない（一覧に出る前は、見つからずに終了コード 4 で止まる。そのときは `youtube-cli ls --json` に出るまで待つ）。

### 上げる前に知っておくこと

- 1 日に上げられるのは 100 本まで（`videos.insert` の上限。プロジェクトごと。終了コード 8）。
- 上げた直後の動画は、処理が終わるまで見られないことがある（`uploadStatus` は `uploaded` → `processed`）。
- 上げた動画は `ls` に、この PC ではすぐ出る（手元の記録のため。[limitations.md](limitations.md)）。

## ls

```bash
youtube-cli ls
youtube-cli ls -l
youtube-cli ls --json
youtube-cli ls --playlist "紹介動画" --json
```

- このツールで上げた動画（目印のタグがあるもの）を、公開日時の新しい順に表示する。
- `--json` なしは 1 行に 1 本、タブ区切りで `タイトル	URL	公開範囲/アップロードの状態`。`-l` を付けると、先頭に ID、最後に公開日時とタグ（カンマ区切り）が付く。
- `--playlist <名前か ID>` で、このツールで作ったその再生リストに入っている動画だけにする。見つからなければ終了コード 4、同じ名前が複数あれば 6。
- `--json` は配列。

```json
[{"id":"abc123","title":"...","url":"https://youtu.be/abc123","privacyStatus":"unlisted","uploadStatus":"processed","publishedAt":"2026-11-15T00:00:00Z","tags":["youtube-cli"]}]
```

- YouTube の一覧は、上げた直後の動画を返すのが遅れることがある。そのため、この PC の手元の記録（`ledger.json`）にある動画の ID を足して引く。**ほかの PC で上げた直後の動画は、遅れて出る**ことがある。手元の記録にあっても YouTube に見つからない動画（Studio で消した動画など）は出ない。

## thumbnail

```bash
youtube-cli thumbnail abc123 ./thumb.png
youtube-cli thumbnail abc123 ./thumb.jpg --json
```

- このツールで上げた動画だけが対象（そうでなければ終了コード 4）。
- 画像は jpg か png で 2 MiB まで（違えば終了コード 2）。
- カスタムサムネイルには、チャンネルの電話番号による確認が要る（[limitations.md](limitations.md)）。
- 結果: `thumbnail	<動画 ID>	<URL>`。`--json` は `{"videoId":"...","url":"..."}`。

## playlist

再生リストは、既定では拒否リストで `playlist-create` と `playlist-add` が止めてある（[policy.md](policy.md)）。使うときは、人が `policy.toml` からその 2 つを消す。拒否リストの名前は `playlist-ls`・`playlist-create`・`playlist-add`。

```bash
youtube-cli playlist create "紹介動画"
youtube-cli playlist ls --json
youtube-cli playlist add abc123 "紹介動画"
```

- `playlist create <名前> [--description <説明>]`: 限定公開の再生リストを作る。説明の最後に目印の行 `[youtube-cli]` を足す。同じ名前の、このツールで作った再生リストがあれば終了コード 5。結果は `created	<名前>	<URL>`。`--json` は `{"id":"...","title":"...","url":"https://www.youtube.com/playlist?list=...","privacyStatus":"unlisted","warnings":[]}`。
- `playlist ls`: このツールで作った再生リストを表示する。1 行は `名前	URL	公開範囲	動画の本数`。`--json` は配列で、各要素は `id`・`title`・`url`・`privacyStatus`・`itemCount`。
- `playlist add <動画 ID> <再生リストの名前か ID>`: このツールで上げた動画を、このツールで作った再生リストに入れる。動画が見つからなければ終了コード 4。再生リストが限定公開でなくなっていれば（Studio で公開や非公開に変えたなど）、入れずに終了コード 7。すでに入っていれば何もしない。結果は `added`（入れた）か `exists`（すでにあった）で始まる `<動作>	<動画 ID>	<再生リスト名>	<URL>`。`--json` は `{"videoId":"...","playlistId":"...","playlistTitle":"...","playlistUrl":"...","added":true}`（`added` が `false` ならすでに入っていた）。
- 再生リストの名前は、同じ名前が複数あれば終了コード 6（候補の ID と URL が出る）。ID で指定する。
- `playlist create` と `playlist add`（`upload --playlist` の再生リストの手順も）が、YouTube の 5xx や通信の失敗で終わったときは、作られた（入った）かもしれないので送り直さず、終了コード 1 で「〜かどうか分かりません」と出る。やり直す前に、`youtube-cli playlist ls`（入れたときは `youtube-cli ls --playlist <再生リスト>`）で確かめる。確かめずに作り直すと、同じ名前の再生リストが 2 つになり、名前で指定すると終了コード 6 になる。
- 再生リストの名前を渡す操作（`ls --playlist`、`playlist add`、`upload --playlist`）は、このツールで作った再生リストの中から探す。

## logout

```bash
youtube-cli logout
```

Google 側でトークンを無効化（revoke）してから、保存したトークンを消す。暗号化の鍵は消さない。`client_secret.enc` が同じ鍵で暗号化されているので、鍵を残しておけば、次の `login` で JSON を指定し直さなくてよい。

## エラーの表示

`--json` なしでは、標準エラーに `エラー: <本文>` と、補足の行（`  - ...`）が出る。`--json` では、標準エラーに 1 つの JSON が出る。原因（API や通信のエラー）があれば、`details` の最後の要素に入る。

```json
{"error":{"kind":"conflict","message":"...","details":["..."]}}
```

## 終了コード

| 終了コード | `kind` | 意味 |
|---|---|---|
| 0 | | 成功 |
| 1 | `unexpected` | 予期しないエラー（通信、API、ローカルの入出力）。`upload` の結果が分からないとき、動画ができたあとのサムネイルや再生リストの失敗、再生リストを作れたか・入れられたかが分からないときもここ |
| 2 | `usage` | 引数の誤り（タイトルが長すぎる、説明に `<` か `>` がある、サムネイルの形式が違う、`setup` を端末以外で動かしたなど） |
| 3 | `setup` | 準備が必要（`init` をしていない、トークンがない、トークンの更新に失敗した、client_secret がない、暗号化の鍵がない・合わない・キーチェーンが使えない、古い形式のファイルだけがある、ログインしたチャンネルが保存してあるチャンネルと違う、アカウントにチャンネルがない、`setup` で gcloud がない・ログインしていない） |
| 4 | `not_found` | 見つからない（手元のファイル、このツールで上げた動画、このツールで作った再生リスト） |
| 5 | `conflict` | 衝突（同じタイトルの動画がある、同じ名前の再生リストがある、使えない状態の既存の動画がある、`login` で別のチャンネルを選んだ） |
| 6 | `ambiguous` | 再生リストの名前があいまい（同じ名前が複数ある） |
| 7 | `denied` | 拒否リストで止められた（拒否リストが読めない場合や、動画を入れようとした再生リストが限定公開でない場合も含む） |
| 8 | `quota` | YouTube API のクォータを使い切った。太平洋時間の 0 時に戻る |

**終了コード 8 のとき。** 1 日に上げられる本数（`videos.insert`）は、プロジェクトごとに 100 本まで。太平洋時間の 0 時（日本時間では 16 時か 17 時。夏時間かどうかで変わる）にならないと戻らない。待ってからやり直す。
