# AI エージェントから使うとき

Claude Code や Codex には、この決まりをまとめたスキルを配っている。[agents/](../agents/README.md) の `setup.sh` で入れられる。

- **`--json` を付ける。** 成功したときは、標準出力に結果の JSON が1つだけ出る。失敗したときは、エラーの JSON（`{"error":{"kind":...,"message":...,"details":[...]}}`）が標準エラーに出る。`upload` で動画ができたあとにサムネイルや再生リストの追加が失敗したときは、結果の JSON が標準出力に、エラーの JSON が標準エラーにと、両方に出る。結果は終了コードと JSON の両方で判断する。`thumbnail.done` や `playlist.done` が `false` の手順は、終わっていない。
- エラーに原因（YouTube API や通信のエラーなど）があれば、`details` の最後の要素に入る（`--json` なしでは、最後の補足の行に出る）。
- `agents/setup.sh` は Windows に対応していない（シンボリックリンクを張る sh のスクリプトのため）。Windows では、`agents/skills/youtube-cli/` を手でコピーして、スキルの置き場所（Claude Code なら `%USERPROFILE%\.claude\skills\youtube-cli`）に入れる。PowerShell なら、リポジトリの直下で `Copy-Item -Recurse agents\skills\youtube-cli "$env:USERPROFILE\.claude\skills\"` を実行する。コピーは自動で更新されないので、リポジトリを更新したら、コピーし直す。
- **作業の前に `youtube-cli status --json`** で、ログインできているかを確かめる。
- **大きな動画のアップロードはバックグラウンドで実行する。** AI エージェントには、コマンドの実行時間に上限があることが多い（Claude Code の Bash ツールは既定で 2 分、最大で 10 分）。Claude Code なら `run_in_background` を使う。途中で止められると、動画ができたかどうかが分からなくなる。そのときは、人に Studio で確かめてもらってから、`upload --skip-existing` で同じコマンドをやり直す。
- **`setup` と `login` は人が行う。** ブラウザでの操作が要り、標準入力も読むコマンドなので、エージェントには実行させない。`setup` は標準入力と標準エラーが端末でなければ、何もせずに終了コード 2 で止まる。さらに、本物の gcloud で GCP を変える（プロジェクトを作る）ので、なおさら人が行う。
- **終了コード 3 が返ったら、人に戻す。** 「`youtube-cli init`、`youtube-cli setup`、`youtube-cli login` のどれかが必要」と伝えて、止まる。エージェントが自分でセットアップやログインをしようとしない。公開ステータスが「テスト」なので、ログインは 7日で切れる。前に使えていたのに終了コード 3 になったときは、たいていこれで、人が `youtube-cli login` をやり直せば済む。
- **終了コード 7（拒否リスト）と 8（クォータ）でも止まって人に伝える。** 7 は、人が `policy.toml` で止めたコマンド。書き換えたり回避したりしない。8 は、1 日の上限（動画は 100 本まで）を使い切ったことで、太平洋時間の 0 時に戻る。
- **終了コード 1 で「アップロードの結果が分かりません」と出たときは、送り直さない。** 動画ができているかもしれない。止まって人に Studio で確かめてもらい、そのあと `--skip-existing` を付けてやり直す。
- **終了コード 5 で、既存の動画が「使えない状態」と出たときも止まる。** youtube-cli には削除がないので、人が Studio で消す。
- 動画の削除・更新・公開範囲の変更のコマンドはなく、公開範囲は常に限定公開なので、取り返しのつかない操作は起きにくい。ただし OAuth スコープは広い（[limitations.md](limitations.md)）ので、設定ディレクトリのファイルをエージェントに渡さない。さらに絞りたいときは、拒否リスト（[policy.md](policy.md)）を使う。
- 動画のタイトルの付け方、再生リストの扱いなどのエージェント向けの決まりは、スキル（`agents/skills/youtube-cli/SKILL.md`）にある。
- macOS では、youtube-cli を入れ直した直後の最初の実行でキーチェーンのダイアログが出る。エージェントに使わせる前に、人が1回 `youtube-cli status` を実行して許可しておく（[setup.md](setup.md#鍵の置き場所)）。
